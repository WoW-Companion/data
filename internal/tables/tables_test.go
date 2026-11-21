package tables

import (
	"reflect"
	"strings"
	"testing"
)

func TestReadCSV(t *testing.T) {
	cases := []struct {
		name, in string
		want     []Row
	}{
		{"empty", "", nil},
		{"header only", "ID,Name\n", nil},
		{"rows and quoting", "ID,Name\n1,\"a, b\"\n2,c\n", []Row{{"ID": "1", "Name": "a, b"}, {"ID": "2", "Name": "c"}}},
		{"short record", "ID,Name\n3\n", []Row{{"ID": "3"}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := ReadCSV(strings.NewReader(c.in))
			if err != nil || !reflect.DeepEqual(got, c.want) {
				t.Fatalf("got %v, %v; want %v", got, err, c.want)
			}
		})
	}
}

func TestStatTypes(t *testing.T) {
	cases := []struct {
		name string
		row  Row
		want []int16
	}{
		{"none", Row{}, []int16{}},
		{"all empty", Row{"StatModifier_bonusStat_0": "-1", "StatModifier_bonusStat_1": "-1"}, []int16{}},
		{"mixed", Row{"StatModifier_bonusStat_0": "3", "StatModifier_bonusStat_1": "-1", "StatModifier_bonusStat_4": "7"}, []int16{3, 7}},
	}
	for _, c := range cases {
		if got := StatTypes(c.row); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}

func TestStatsStatusOf(t *testing.T) {
	cases := []struct {
		name                 string
		stats                []int16
		class, slot, quality int
		want                 string
	}{
		{"listed", []int16{3}, 4, 5, 3, StatsKnown},
		{"armor without stats", nil, 4, 5, 3, StatsUnknown},
		{"weapon without stats", nil, 2, 13, 2, StatsUnknown},
		{"common armor", nil, 4, 5, 1, StatsNone},
		{"no slot", nil, 4, 0, 4, StatsNone},
		{"consumable", nil, 0, 0, 3, StatsNone},
	}
	for _, c := range cases {
		if got := StatsStatusOf(c.stats, c.class, c.slot, c.quality); got != c.want {
			t.Errorf("%s: got %s want %s", c.name, got, c.want)
		}
	}
}

func TestBuildItems(t *testing.T) {
	sparse := []Row{
		{"ID": "20", "Display_lang": "Plain Vest", "InventoryType": "5", "OverallQualityID": "3", "ItemLevel": "40"},
		{"ID": "10", "Display_lang": "Ring", "InventoryType": "11", "OverallQualityID": "2", "StatModifier_bonusStat_0": "5", "StatModifier_bonusStat_1": "-1", "Description_lang": "never kept"},
	}
	base := []Row{{"ID": "20", "ClassID": "4", "SubclassID": "1"}, {"ID": "10", "ClassID": "4", "SubclassID": "0"}}
	got := BuildItems(sparse, base)
	if len(got) != 2 || got[0].ID != 10 || got[1].ID != 20 {
		t.Fatalf("not sorted by id: %+v", got)
	}
	if got[0].StatsStatus != StatsKnown || !reflect.DeepEqual(got[0].StatTypes, []int16{5}) {
		t.Errorf("ring: %+v", got[0])
	}
	if got[1].StatsStatus != StatsUnknown || got[1].AllowableClassMask != -1 || got[1].SubclassID != 1 {
		t.Errorf("vest: %+v", got[1])
	}
}

func TestBuildQuests(t *testing.T) {
	quests := []Row{{"ID": "9", "UniqueBitFlag": "2"}, {"ID": "3", "UiQuestDetailsThemeID": "4"}}
	lines := []Row{{"ID": "2", "QuestID": "3", "QuestLineID": "88"}, {"ID": "1", "QuestID": "3", "QuestLineID": "77"}}
	pois := []Row{{"ID": "5", "QuestID": "3", "MapID": "1", "UiMapID": "1411"}}
	got := BuildQuests(quests, lines, pois)
	if len(got) != 2 || got[0].ID != 3 || got[1].ID != 9 {
		t.Fatalf("not sorted by id: %+v", got)
	}
	q := got[0]
	if q.UIThemeID != 4 || q.QuestLineID == nil || *q.QuestLineID != 77 || q.MapID == nil || *q.MapID != 1 || *q.UIMapID != 1411 {
		t.Errorf("quest 3 (first line by ID wins): %+v", q)
	}
	if got[1].UniqueBitFlag != 2 || got[1].QuestLineID != nil || got[1].MapID != nil {
		t.Errorf("quest 9 should have nulls: %+v", got[1])
	}
}

func TestBuildNpcs(t *testing.T) {
	creatures := []Row{
		{"ID": "30", "Name_lang": "Boss", "Classification": "3", "CreatureType": "7", "Title_lang": "never kept"},
		{"ID": "10", "Name_lang": "Wolf", "Classification": "0", "CreatureType": "1", "CreatureFamily": "1"},
		{"ID": "20", "Name_lang": "Bare"},
	}
	diffs := []Row{
		{"ID": "2", "CreatureID": "10", "MinLevel": "4", "MaxLevel": "8", "FactionTemplateID": "14"},
		{"ID": "1", "CreatureID": "10", "MinLevel": "3", "MaxLevel": "5", "FactionTemplateID": "0"},
		{"ID": "3", "CreatureID": "30"},
		{"ID": "4", "CreatureID": "20", "MinLevel": "1", "MaxLevel": "0"},
	}
	got := BuildNpcs(creatures, diffs)
	if len(got) != 3 || got[0].ID != 10 || got[1].ID != 20 || got[2].ID != 30 {
		t.Fatalf("not sorted by id: %+v", got)
	}
	w := got[0]
	if w.Name != "Wolf" || w.FamilyID != 1 || w.FactionTemplateID == nil || *w.FactionTemplateID != 14 || *w.MinLevel != 3 || *w.MaxLevel != 8 {
		t.Errorf("wolf: %+v", w)
	}
	if got[1].MinLevel != nil || got[1].FactionTemplateID != nil {
		t.Errorf("max level 0 means no range: %+v", got[1])
	}
	if b := got[2]; b.Classification != 3 || b.CreatureType != 7 || b.MinLevel != nil {
		t.Errorf("boss: %+v", b)
	}
}

func TestReadTableRequiredColumns(t *testing.T) {
	cases := []struct {
		name, table, in, wantErr string
	}{
		{"all present", "Item", "ID,ClassID,SubclassID\n1,2,3\n", ""},
		{"extra columns fine", "Item", "ID,ClassID,SubclassID,Other\n", ""},
		{"renamed column", "Creature", "ID,Name,Classification,CreatureType,CreatureFamily\n", `Creature: required column "Name_lang" missing`},
		{"missing in header-only file", "QuestV2", "ID,UniqueBitFlag\n", `QuestV2: required column "UiQuestDetailsThemeID" missing`},
		{"empty file", "Item", "", `Item: required column "ID" missing`},
	}
	for _, c := range cases {
		_, err := ReadTable(c.table, strings.NewReader(c.in))
		if (err == nil) != (c.wantErr == "") || (err != nil && !strings.Contains(err.Error(), c.wantErr)) {
			t.Errorf("%s: err %v, want %q", c.name, err, c.wantErr)
		}
	}
}

func talentFixture() TalentInput {
	return TalentInput{
		Groups:       []Row{{"ID": "20", "TraitTreeID": "1"}, {"ID": "10", "TraitTreeID": "1"}, {"ID": "30", "TraitTreeID": "2"}},
		Conds:        []Row{{"TraitNodeGroupID": "10", "SpentAmountRequired": "30"}, {"TraitNodeGroupID": "0", "SpentAmountRequired": "0"}},
		SkillTrees:   []Row{{"SkillLineID": "613", "TraitTreeID": "1"}},
		SkillClasses: []Row{{"SkillID": "613", "ClassMask": "16"}},
		Classes:      []Row{{"ID": "5", "Name_lang": "Priest"}, {"ID": "1", "Name_lang": "Warrior"}},
		Nodes: []Row{
			{"ID": "7", "TraitTreeID": "1"}, {"ID": "5", "TraitTreeID": "1"}, {"ID": "6", "TraitTreeID": "2"},
		},
		GroupNodes: []Row{{"TraitNodeGroupID": "20", "TraitNodeID": "5"}, {"TraitNodeGroupID": "10", "TraitNodeID": "5"}},
		NodeEntries: []Row{
			{"TraitNodeID": "5", "TraitNodeEntryID": "51", "_Index": "1"}, {"TraitNodeID": "5", "TraitNodeEntryID": "50", "_Index": "0"},
			{"TraitNodeID": "7", "TraitNodeEntryID": "70", "_Index": "0"},
		},
		Entries:     []Row{{"ID": "50", "TraitDefinitionID": "500", "MaxRanks": "2"}, {"ID": "70", "TraitDefinitionID": "700", "MaxRanks": "1"}},
		Definitions: []Row{{"ID": "500", "SpellID": "15473"}, {"ID": "700", "SpellID": "9", "OverrideName_lang": "Custom"}},
		Edges:       []Row{{"LeftTraitNodeID": "5", "RightTraitNodeID": "7"}, {"LeftTraitNodeID": "5", "RightTraitNodeID": "7"}},
		SpellNames:  []Row{{"ID": "15473", "Name_lang": "Shadowform"}},
	}
}

func TestBuildTalentTrees(t *testing.T) {
	got := BuildTalentTrees(talentFixture())
	if len(got) != 3 || got[0].GroupID != 10 || got[1].GroupID != 20 || got[2].GroupID != 30 {
		t.Fatalf("order: %+v", got)
	}
	g := got[0]
	if g.ClassID == nil || *g.ClassID != 5 || *g.ClassName != "Priest" || g.GroupOrder != 0 || g.RequiredPoints == nil || *g.RequiredPoints != 30 || g.GroupName != nil {
		t.Errorf("group 10: %+v", g)
	}
	if got[1].GroupOrder != 1 || got[1].RequiredPoints != nil {
		t.Errorf("group 20: %+v", got[1])
	}
	if got[2].ClassID != nil || got[2].GroupOrder != 0 {
		t.Errorf("tree without class: %+v", got[2])
	}
}

func TestBuildTalentNodes(t *testing.T) {
	got := BuildTalentNodes(talentFixture())
	if len(got) != 3 || got[0].NodeID != 5 || got[1].NodeID != 6 || got[2].NodeID != 7 {
		t.Fatalf("order: %+v", got)
	}
	n := got[0]
	if n.SpellID == nil || *n.SpellID != 15473 || *n.Name != "Shadowform" || *n.MaxRank != 2 || !reflect.DeepEqual(n.GroupIDs, []int32{10, 20}) {
		t.Errorf("node 5 (first entry by index, name from spell): %+v", n)
	}
	if got[1].SpellID != nil || got[1].Name != nil || got[1].MaxRank != nil || len(got[1].GroupIDs) != 0 {
		t.Errorf("node without entry: %+v", got[1])
	}
	if *got[2].Name != "Custom" || !reflect.DeepEqual(got[2].PrerequisiteNodeIDs, []int32{5}) {
		t.Errorf("node 7 (override name, deduped prerequisite): %+v", got[2])
	}
}
