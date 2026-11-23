package tables

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/WoW-Companion/data/internal/fetch"
)

func sp(s string) *string { return &s }
func mr(v int16) *int16   { return &v }

func buildFixture() ([]TalentTree, []TalentNode) {
	priest := int8(5)
	trees := []TalentTree{{TreeID: 1, ClassID: &priest, ClassName: sp("Priest"), GroupID: 10}, {TreeID: 2, ClassName: sp("Rogue"), GroupID: 30}}
	nodes := []TalentNode{
		{NodeID: 1, TreeID: 1, GroupIDs: []int32{10}, Name: sp("Alpha"), MaxRank: mr(3)},
		{NodeID: 2, TreeID: 1, GroupIDs: []int32{10}, Name: sp("Beta"), MaxRank: mr(1), PrerequisiteNodeIDs: []int32{1}},
		{NodeID: 3, TreeID: 1, GroupIDs: []int32{11}, Name: sp("Other group"), MaxRank: mr(1)},
		{NodeID: 4, TreeID: 2, GroupIDs: []int32{10}, Name: sp("Other tree"), MaxRank: mr(1)},
	}
	return trees, nodes
}

const goodBuild = `# comment
class: Priest
spec: "Test: Spec"
group_id: 10
status: "advisor default, not authoritative"
bands:
  - levels: 10-19
    rationale: "Because: reasons."
    steps:
      - {node: 1, name: Alpha, points: 3}
      - {node: 2, name: Beta, points: 1}
  - levels: 20-29
    rationale: more
    steps:
      - {node: 1, name: Alpha, points: 0}
`

func TestParseTalentBuild(t *testing.T) {
	b, err := ParseTalentBuild("f.yaml", goodBuild)
	if err != nil {
		t.Fatal(err)
	}
	if b.Class != "Priest" || b.Spec != "Test: Spec" || b.GroupID != 10 || len(b.Bands) != 2 || b.Bands[0].Low != 10 || b.Bands[0].High != 19 ||
		b.Bands[0].Rationale != "Because: reasons." || len(b.Bands[0].Steps) != 2 || b.Bands[0].Steps[1] != (BuildStep{2, "Beta", 1}) {
		t.Errorf("parsed: %+v", b)
	}
	cases := []struct{ name, in, want string }{
		{"unknown key", "colour: red\n", "unknown key"},
		{"bad levels", "bands:\n  - levels: ten\n", "not LOW-HIGH"},
		{"step not a map", "bands:\n  - levels: 10-19\n    steps:\n      - node 1\n", "flow map"},
		{"step field missing", "bands:\n  - levels: 10-19\n    steps:\n      - {node: 1, points: 2}\n", "needs node, name and points"},
		{"step number", "bands:\n  - levels: 10-19\n    steps:\n      - {node: x, name: A, points: 2}\n", "not a number"},
		{"indent outside bands", "class: A\n  oops: 1\n", "outside bands"},
	}
	for _, c := range cases {
		if _, err := ParseTalentBuild("f.yaml", c.in); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err %v, want %q", c.name, err, c.want)
		}
	}
}

func TestValidateTalentBuild(t *testing.T) {
	trees, nodes := buildFixture()
	good, _ := ParseTalentBuild("f.yaml", goodBuild)
	good.Bands = good.Bands[:1]
	if errs := ValidateTalentBuild(good, trees, nodes); len(errs) != 0 {
		t.Fatalf("good build: %v", errs)
	}
	cases := []struct {
		name string
		edit func(b *TalentBuild)
		want string
	}{
		{"unknown group", func(b *TalentBuild) { b.GroupID = 99 }, "not a group of a Priest tree"},
		{"wrong class", func(b *TalentBuild) { b.Class = "Rogue" }, "not a group of a Rogue tree"},
		{"not marked default", func(b *TalentBuild) { b.Status = "final" }, "advisor default"},
		{"unknown node", func(b *TalentBuild) { b.Bands[0].Steps[0].NodeID = 77 }, "not in the talent tables"},
		{"node in other group", func(b *TalentBuild) { b.Bands[0].Steps[0] = BuildStep{3, "Other group", 1} }, "not in group 10"},
		{"node in other tree", func(b *TalentBuild) { b.Bands[0].Steps[0] = BuildStep{4, "Other tree", 1} }, "not the spec's tree"},
		{"wrong name", func(b *TalentBuild) { b.Bands[0].Steps[0].Name = "Gamma" }, `named "Alpha" in the tables`},
		{"zero points", func(b *TalentBuild) { b.Bands[0].Steps[0].Points = 0 }, "takes 0 points"},
		{"over max rank", func(b *TalentBuild) { b.Bands[0].Steps = append(b.Bands[0].Steps, BuildStep{1, "Alpha", 1}) }, "reaches 4 points, max is 3"},
		{"over band points", func(b *TalentBuild) { b.Bands[0].High = 12 }, "4 points spent, 3 available at level 12"},
		{"prerequisite not maxed", func(b *TalentBuild) { b.Bands[0].Steps[0].Points = 2 }, "needs prerequisite 1 (Alpha) at max rank"},
		{"prerequisite later", func(b *TalentBuild) {
			b.Bands[0].Steps[0], b.Bands[0].Steps[1] = b.Bands[0].Steps[1], b.Bands[0].Steps[0]
		}, "needs prerequisite"},
		{"gap between bands", func(b *TalentBuild) {
			b.Bands = append(b.Bands, BuildBand{Low: 30, High: 39, Rationale: "r", Steps: []BuildStep{{1, "Alpha", 1}}})
		}, "does not follow"},
		{"no rationale", func(b *TalentBuild) { b.Bands[0].Rationale = " " }, "rationale is empty"},
	}
	for _, c := range cases {
		b := good
		b.Bands = []BuildBand{good.Bands[0]}
		b.Bands[0].Steps = append([]BuildStep(nil), good.Bands[0].Steps...)
		c.edit(&b)
		errs := strings.Join(ValidateTalentBuild(b, trees, nodes), "\n")
		if !strings.Contains(errs, c.want) {
			t.Errorf("%s: errors %q, want %q", c.name, errs, c.want)
		}
	}
	if PointsAt(9) != 0 || PointsAt(10) != 1 || PointsAt(39) != 30 {
		t.Error("PointsAt")
	}
}

func TestBuildTalentBuildsRows(t *testing.T) {
	trees, nodes := buildFixture()
	good, _ := ParseTalentBuild("f.yaml", goodBuild)
	good.Bands = good.Bands[:1]
	rows, err := BuildTalentBuilds([]TalentBuild{good}, trees, nodes)
	if err != nil || len(rows) != 2 || rows[1].Order != 2 || rows[1].Band != "10-19" || rows[1].Source != "curated (advisor defaults)" || rows[0].Spec != "Test: Spec" {
		t.Fatalf("rows %+v err %v", rows, err)
	}
	good.Bands[0].Steps[0].NodeID = 77
	if _, err := BuildTalentBuilds([]TalentBuild{good}, trees, nodes); err == nil {
		t.Error("invalid build accepted")
	}
}

// The committed files parse, and each carries its own header: no network needed.
func TestCuratedTalentBuildsParse(t *testing.T) {
	builds, err := LoadTalentBuilds(filepath.Join("..", "..", "curated", "talents"))
	if err != nil || len(builds) != 15 {
		t.Fatalf("%d builds, err %v", len(builds), err)
	}
	for _, b := range builds {
		if len(b.Bands) < 3 || b.Spec == "" || !strings.Contains(b.Status, "advisor default") {
			t.Errorf("%s: %+v", b.File, b)
		}
	}
}

// With a cached build (after make fetch) the committed builds are validated against the
// real talent tables; without one the test is skipped, so make verify needs no network.
func TestCuratedTalentBuildsAgainstCache(t *testing.T) {
	dir := filepath.Join("..", "..", "dist", "cache", fetch.DefaultBuild)
	if _, err := os.Stat(dir); err != nil {
		t.Skip("no cached build; run make fetch")
	}
	c, err := LoadCache(dir)
	if err != nil {
		t.Fatal(err)
	}
	builds, err := LoadTalentBuilds(filepath.Join("..", "..", "curated", "talents"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := BuildTalentBuilds(builds, c.TalentTrees, c.TalentNodes); err != nil {
		t.Error(err)
	}
}
