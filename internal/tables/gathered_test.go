package tables

import (
	"reflect"
	"testing"

	"github.com/WoW-Companion/data/internal/everloot"
)

func TestBuildNodes(t *testing.T) {
	types := []Row{{"ID": "1", "Name_lang": "Pick Lock"}, {"ID": "2", "Name_lang": "Herbalism"}, {"ID": "3", "Name_lang": "Mining"}}
	locks := []Row{
		{"ID": "10", "_Index_0": "2", "Type_0": "2", "Skill_0": "15"},
		{"ID": "11", "_Index_0": "1", "Type_0": "2", "Skill_0": "25", "_Index_1": "3", "Type_1": "2", "Skill_1": "65"},
		{"ID": "12", "_Index_0": "3", "Type_0": "1", "Skill_0": "99"}, // an item key, not a skill
		{"ID": "13", "_Index_0": "1", "Type_0": "2", "Skill_0": "25"},
	}
	objs := func(id, typ, lock, name string) Row {
		return Row{"ID": id, "TypeID": typ, "PropValue_0": lock, "Name_lang": name}
	}
	cases := []struct {
		name string
		obj  Row
		want *Node
	}{
		{"herb", objs("100", "3", "10", "Peacebloom"), &Node{100, "Peacebloom", "Herbalism", 15}},
		{"ore in a later slot", objs("101", "3", "11", ""), &Node{101, "", "Mining", 65}},
		{"not a chest", objs("102", "5", "10", "Doodad"), nil},
		{"lockpick only", objs("103", "3", "13", "Chest"), nil},
		{"item key not skill", objs("104", "3", "12", "Chest"), nil},
		{"unknown lock", objs("105", "3", "99", "Chest"), nil},
	}
	for _, c := range cases {
		got := BuildNodes(GatherInput{GameObjects: []Row{c.obj}, Locks: locks, LockTypes: types})
		var want map[int32]Node
		if c.want != nil {
			want = map[int32]Node{c.want.ID: *c.want}
		} else {
			want = map[int32]Node{}
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s: got %+v, want %+v", c.name, got, want)
		}
	}
}

func TestApplyGathered(t *testing.T) {
	rows := []everloot.LootSource{
		{ItemID: 2770, SourceKind: everloot.KindObject, SourceID: 1731, Kills: 31, Drops: 31, Rate: 1},
		{ItemID: 2770, SourceKind: everloot.KindObject, SourceID: 9999, Kills: 4, Drops: 2, Rate: 0.5},
		{ItemID: 2770, SourceKind: everloot.KindCreature, SourceID: 1731},
	}
	nodes := map[int32]Node{1731: {ID: 1731, Name: "Copper Vein", Profession: "Mining", SkillRequired: 1}}
	if n := ApplyGathered(rows, nodes); n != 1 {
		t.Fatalf("changed %d rows, want 1", n)
	}
	var g *everloot.LootSource
	kinds := map[string]int{}
	for i := range rows {
		kinds[rows[i].SourceKind]++
		if rows[i].SourceKind == everloot.KindGathered {
			g = &rows[i]
		}
	}
	if kinds[everloot.KindObject] != 1 || kinds[everloot.KindCreature] != 1 || g == nil {
		t.Fatalf("kinds %v", kinds)
	}
	if g.Kills != 31 || g.NodeID == nil || *g.NodeID != 1731 || *g.NodeName != "Copper Vein" || *g.Profession != "Mining" || *g.SkillRequired != 1 {
		t.Errorf("gathered row %+v", g)
	}
}
