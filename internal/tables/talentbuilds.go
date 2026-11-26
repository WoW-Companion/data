package tables

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// BuildSource is the source column of every talent_builds row.
const BuildSource = "curated (unverified)"

// IcyVeinsSource is the source of a row read from an Icy Veins WoW Forever leveling guide.
const IcyVeinsSource = "icy-veins.com WoW Forever leveling guides"

// FirstTalentLevel is the level of the first talent point. The client tables fetched here
// carry no currency rules, so the points available at a level are an assumption: one point
// per level from FirstTalentLevel on, so level L has L-FirstTalentLevel+1 points.
const FirstTalentLevel = 10

// PointsAt is the talent points a character has at a level, under the assumption above.
func PointsAt(level int) int { return max(0, level-FirstTalentLevel+1) }

// TalentBuild is one curated file: the build of one spec, in level bands.
type TalentBuild struct {
	Class   string
	Spec    string
	GroupID int32 // the talent group of the spec's tree
	Status  string
	Bands   []BuildBand
	File    string
}

// BuildBand is the points added in one level band, in the order to spend them.
type BuildBand struct {
	Low, High int
	Rationale string
	Steps     []BuildStep
}

// BuildStep spends Points more ranks of one node. Name is checked against the node's name.
type BuildStep struct {
	NodeID int32
	Name   string
	Points int
}

// TalentBuildRow is one row of the talent_builds table. Order counts a spec's steps from 1
// across all its bands.
type TalentBuildRow struct {
	Class     string `parquet:"class"`
	Spec      string `parquet:"spec"`
	Band      string `parquet:"band"`
	Order     int16  `parquet:"order"`
	NodeID    int32  `parquet:"node_id"`
	Points    int16  `parquet:"points"`
	Rationale string `parquet:"rationale"`
	Source    string `parquet:"source"`
	// The columns below are filled only for rows read from an Icy Veins guide (one row per
	// point, Points 1); null on a curated (unverified) row.
	Level      *int16  `parquet:"level,optional"`
	TalentName *string `parquet:"talent_name,optional"`
	RankAfter  *int16  `parquet:"rank_after,optional"`
	SourceURL  *string `parquet:"source_url,optional"`
	FetchedAt  *string `parquet:"fetched_at,optional"`
}

// unquote strips a pair of double quotes, or returns the trimmed text.
func unquote(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		if u, err := strconv.Unquote(s); err == nil {
			return u
		}
		return s[1 : len(s)-1]
	}
	return s
}

// splitTop splits on sep outside double quotes.
func splitTop(s string, sep byte) []string {
	var out []string
	in, start := false, 0
	for i := 0; i < len(s); i++ {
		switch {
		case s[i] == '\\' && in:
			i++
		case s[i] == '"':
			in = !in
		case s[i] == sep && !in:
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	return append(out, s[start:])
}

// keyValue splits "key: value" at the first colon outside quotes.
func keyValue(s string) (string, string, bool) {
	p := splitTop(s, ':')
	if len(p) < 2 {
		return "", "", false
	}
	return strings.TrimSpace(p[0]), strings.TrimSpace(strings.Join(p[1:], ":")), true
}

// ParseTalentBuild reads the small YAML subset the curated files use: top-level scalars,
// then "bands:" holding "- levels: 10-19" items with a rationale scalar and a "steps:" list
// of flow maps "- {node: 1, name: X, points: 2}". Full-line comments and blank lines are
// skipped; strings may be double-quoted.
func ParseTalentBuild(file string, text string) (TalentBuild, error) {
	b := TalentBuild{File: file}
	var band *BuildBand
	inBands, inSteps := false, false
	fail := func(n int, format string, a ...any) (TalentBuild, error) {
		return b, fmt.Errorf("%s:%d: %s", file, n, fmt.Sprintf(format, a...))
	}
	for i, raw := range strings.Split(text, "\n") {
		n := i + 1
		line := strings.TrimRight(raw, " \t\r")
		trim := strings.TrimSpace(line)
		if trim == "" || strings.HasPrefix(trim, "#") {
			continue
		}
		indent := len(line) - len(strings.TrimLeft(line, " "))
		switch {
		case indent == 0:
			inBands, inSteps = false, false
			k, v, ok := keyValue(trim)
			if !ok {
				return fail(n, "want key: value")
			}
			switch k {
			case "class":
				b.Class = unquote(v)
			case "spec":
				b.Spec = unquote(v)
			case "status":
				b.Status = unquote(v)
			case "group_id":
				id, err := strconv.Atoi(v)
				if err != nil {
					return fail(n, "group_id %q is not a number", v)
				}
				b.GroupID = int32(id)
			case "bands":
				inBands = true
			default:
				return fail(n, "unknown key %q", k)
			}
		case !inBands:
			return fail(n, "indented line outside bands")
		case strings.HasPrefix(trim, "- ") && indent == 2:
			inSteps = false
			b.Bands = append(b.Bands, BuildBand{})
			band = &b.Bands[len(b.Bands)-1]
			k, v, ok := keyValue(trim[2:])
			if !ok || k != "levels" {
				return fail(n, "a band starts with levels: LOW-HIGH")
			}
			lo, hi, ok := strings.Cut(unquote(v), "-")
			l, e1 := strconv.Atoi(lo)
			h, e2 := strconv.Atoi(hi)
			if !ok || e1 != nil || e2 != nil {
				return fail(n, "levels %q is not LOW-HIGH", v)
			}
			band.Low, band.High = l, h
		case band == nil:
			return fail(n, "expected a band item")
		case strings.HasPrefix(trim, "- ") && inSteps:
			st, err := parseStep(strings.TrimSpace(trim[2:]))
			if err != nil {
				return fail(n, "%v", err)
			}
			band.Steps = append(band.Steps, st)
		default:
			k, v, ok := keyValue(trim)
			switch {
			case ok && k == "rationale":
				band.Rationale = unquote(v)
			case ok && k == "steps" && v == "":
				inSteps = true
			default:
				return fail(n, "unexpected line in band")
			}
		}
	}
	return b, nil
}

func parseStep(s string) (BuildStep, error) {
	var st BuildStep
	if !strings.HasPrefix(s, "{") || !strings.HasSuffix(s, "}") {
		return st, fmt.Errorf("step must be a flow map {node: N, name: X, points: P}")
	}
	seen := map[string]bool{}
	for _, f := range splitTop(s[1:len(s)-1], ',') {
		k, v, ok := keyValue(f)
		if !ok {
			return st, fmt.Errorf("step field %q is not key: value", strings.TrimSpace(f))
		}
		seen[k] = true
		switch k {
		case "node", "points":
			x, err := strconv.Atoi(v)
			if err != nil {
				return st, fmt.Errorf("step %s %q is not a number", k, v)
			}
			if k == "node" {
				st.NodeID = int32(x)
			} else {
				st.Points = x
			}
		case "name":
			st.Name = unquote(v)
		default:
			return st, fmt.Errorf("unknown step field %q", k)
		}
	}
	if !seen["node"] || !seen["points"] || !seen["name"] {
		return st, fmt.Errorf("step needs node, name and points")
	}
	return st, nil
}

// LoadTalentBuilds parses every curated/talents/<class>/<spec>.yaml under dir, in path order.
func LoadTalentBuilds(dir string) ([]TalentBuild, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*", "*.yaml"))
	if err != nil {
		return nil, err
	}
	slices.Sort(files)
	var out []TalentBuild
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		b, err := ParseTalentBuild(f, string(raw))
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, nil
}

// ValidateTalentBuild checks one build against the talent tables and returns every problem:
//   - the class and the spec's group exist, and the group belongs to the class's tree;
//   - bands are well formed, adjacent and carry a rationale; the status marks an advisor default;
//   - each step names a node of that group, with the node's name, and 1 or more points;
//   - a node's points never exceed its max rank;
//   - the points spent through a band never exceed PointsAt its top level;
//   - a node with prerequisites takes its first point only once every prerequisite is at max rank.
func ValidateTalentBuild(b TalentBuild, trees []TalentTree, nodes []TalentNode) []string {
	var errs []string
	bad := func(format string, a ...any) { errs = append(errs, b.File+": "+fmt.Sprintf(format, a...)) }
	var tree int32 = -1
	for _, t := range trees {
		if t.GroupID == b.GroupID && t.ClassName != nil && *t.ClassName == b.Class {
			tree = t.TreeID
		}
	}
	if tree < 0 {
		bad("group %d is not a group of a %s tree", b.GroupID, b.Class)
	}
	if b.Spec == "" {
		bad("spec is empty")
	}
	if !strings.Contains(strings.ToLower(b.Status), "advisor default") {
		bad("status must say the build is an advisor default, not authoritative")
	}
	byID := map[int32]TalentNode{}
	for _, n := range nodes {
		byID[n.NodeID] = n
	}
	rank := map[int32]int{}
	spent := 0
	for i, band := range b.Bands {
		label := fmt.Sprintf("band %d-%d", band.Low, band.High)
		if band.Low > band.High || band.Low < FirstTalentLevel {
			bad("%s: bad level range", label)
		}
		if i > 0 && band.Low != b.Bands[i-1].High+1 {
			bad("%s does not follow the band before it", label)
		}
		if strings.TrimSpace(band.Rationale) == "" {
			bad("%s: rationale is empty", label)
		}
		if len(band.Steps) == 0 {
			bad("%s: no steps", label)
		}
		for _, s := range band.Steps {
			n, ok := byID[s.NodeID]
			switch {
			case !ok:
				bad("%s: node %d is not in the talent tables", label, s.NodeID)
				continue
			case tree >= 0 && n.TreeID != tree:
				bad("%s: node %d (%s) is in tree %d, not the spec's tree %d", label, s.NodeID, s.Name, n.TreeID, tree)
			case !slices.Contains(n.GroupIDs, b.GroupID):
				bad("%s: node %d (%s) is not in group %d", label, s.NodeID, s.Name, b.GroupID)
			}
			if n.Name == nil || *n.Name != s.Name {
				bad("%s: node %d is named %q in the tables, not %q", label, s.NodeID, deref(n.Name), s.Name)
			}
			if s.Points < 1 {
				bad("%s: node %d (%s) takes %d points", label, s.NodeID, s.Name, s.Points)
				continue
			}
			if n.MaxRank == nil {
				bad("%s: node %d (%s) has no max rank", label, s.NodeID, s.Name)
				continue
			}
			if rank[s.NodeID] == 0 {
				for _, p := range n.PrerequisiteNodeIDs {
					if pn := byID[p]; pn.MaxRank == nil || rank[p] < int(*pn.MaxRank) {
						bad("%s: node %d (%s) needs prerequisite %d (%s) at max rank first", label, s.NodeID, s.Name, p, deref(pn.Name))
					}
				}
			}
			rank[s.NodeID] += s.Points
			spent += s.Points
			if rank[s.NodeID] > int(*n.MaxRank) {
				bad("%s: node %d (%s) reaches %d points, max is %d", label, s.NodeID, s.Name, rank[s.NodeID], *n.MaxRank)
			}
		}
		if avail := PointsAt(band.High); spent > avail {
			bad("%s: %d points spent, %d available at level %d", label, spent, avail, band.High)
		}
	}
	return errs
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// BuildTalentBuilds validates every build and flattens them into talent_builds rows. Every
// problem of every file comes back in one error.
func BuildTalentBuilds(builds []TalentBuild, trees []TalentTree, nodes []TalentNode) ([]TalentBuildRow, error) {
	var errs []string
	var rows []TalentBuildRow
	for _, b := range builds {
		errs = append(errs, ValidateTalentBuild(b, trees, nodes)...)
		order := int16(0)
		for _, band := range b.Bands {
			for _, s := range band.Steps {
				order++
				rows = append(rows, TalentBuildRow{
					Class: b.Class, Spec: b.Spec, Band: fmt.Sprintf("%d-%d", band.Low, band.High), Order: order,
					NodeID: s.NodeID, Points: int16(s.Points), Rationale: band.Rationale, Source: BuildSource,
				})
			}
		}
	}
	if len(errs) > 0 {
		return nil, fmt.Errorf("talent builds invalid:\n%s", strings.Join(errs, "\n"))
	}
	return rows, nil
}
