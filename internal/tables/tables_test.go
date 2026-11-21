package tables

import (
	"reflect"
	"strings"
	"testing"

	"github.com/WoW-Companion/data/internal/everloot"
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

func TestItemStats(t *testing.T) {
	cases := []struct {
		name string
		row  Row
		want []ItemStat
	}{
		{"none", Row{}, []ItemStat{}},
		{"order kept, empty slots skipped, value never inferred", Row{"StatModifier_bonusStat_0": "7", "StatModifier_bonusStat_1": "-1", "StatModifier_bonusStat_2": "3", "StatPercentEditor_0": "10000"}, []ItemStat{{Type: 7}, {Type: 3}}},
	}
	for _, c := range cases {
		if got := ItemStats(c.row); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}

func TestSpeedMS(t *testing.T) {
	cases := []struct {
		name  string
		row   Row
		class int
		want  *int32
	}{
		{"weapon", Row{"ItemDelay": "2600"}, 2, ptr(2600)},
		{"weapon zero delay", Row{"ItemDelay": "0"}, 2, nil},
		{"weapon column absent", Row{}, 2, nil},
		{"armor ignores delay", Row{"ItemDelay": "2600"}, 4, nil},
	}
	for _, c := range cases {
		if got := SpeedMS(c.row, c.class); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}

func TestBuildItems(t *testing.T) {
	sparse := []Row{
		{"ID": "20", "Display_lang": "Plain Vest", "InventoryType": "5", "OverallQualityID": "3", "ItemLevel": "40", "SellPrice": "1250", "BuyPrice": "0"},
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
	if len(got[0].Stats) != 1 || got[0].Stats[0].Type != 5 || got[0].Stats[0].Value != nil || got[0].Armor != nil || got[0].DmgMin != nil || got[0].DmgMax != nil || got[0].SpeedMS != nil {
		t.Errorf("ring stats: %+v", got[0])
	}
	if got[0].SellPrice != nil || got[0].BuyPrice != nil || got[1].SellPrice == nil || *got[1].SellPrice != 1250 || got[1].BuyPrice != nil {
		t.Errorf("prices: %+v %+v", got[0], got[1])
	}
	if got[1].StatsStatus != StatsUnknown || got[1].AllowableClassMask != -1 || got[1].SubclassID != 1 {
		t.Errorf("vest: %+v", got[1])
	}
}

func TestBuildQuests(t *testing.T) {
	quests := []Row{{"ID": "9", "UniqueBitFlag": "2"}, {"ID": "3", "UiQuestDetailsThemeID": "4"}}
	lines := []Row{{"ID": "2", "QuestID": "3", "QuestLineID": "88"}, {"ID": "1", "QuestID": "3", "QuestLineID": "77"}}
	pois := []Row{{"ID": "5", "QuestID": "3", "MapID": "1", "UiMapID": "1411"}}
	uiMaps := []Row{{"ID": "1411", "Name_lang": "Durotar"}, {"ID": "1412", "Name_lang": ""}}
	got := BuildQuests(quests, lines, pois, uiMaps)
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

func TestQuestZone(t *testing.T) {
	quests := []Row{{"ID": "1"}, {"ID": "2"}, {"ID": "3"}, {"ID": "4"}}
	pois := []Row{
		{"ID": "1", "QuestID": "1", "UiMapID": "1411"},
		{"ID": "2", "QuestID": "2", "UiMapID": "1412"},
		{"ID": "3", "QuestID": "3", "UiMapID": "0"},
	}
	uiMaps := []Row{{"ID": "1411", "Name_lang": "Durotar"}, {"ID": "1412", "Name_lang": ""}}
	cases := []struct {
		name     string
		i        int
		id       int32
		hasID    bool
		wantName string
	}{
		{"named zone", 0, 1411, true, "Durotar"},
		{"map without a name", 1, 1412, true, ""},
		{"POI with no UiMap", 2, 0, false, ""},
		{"no POI", 3, 0, false, ""},
	}
	got := BuildQuests(quests, nil, pois, uiMaps)
	for _, c := range cases {
		q := got[c.i]
		if (q.ZoneID != nil) != c.hasID || (c.hasID && *q.ZoneID != c.id) {
			t.Errorf("%s: zone id %v", c.name, q.ZoneID)
		}
		if (q.ZoneName != nil) != (c.wantName != "") || (c.wantName != "" && *q.ZoneName != c.wantName) {
			t.Errorf("%s: zone name %v", c.name, q.ZoneName)
		}
		if q.Level != nil || q.MinLevel != nil {
			t.Errorf("%s: level must stay null: %+v", c.name, q)
		}
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
		{"item delay missing", "ItemSparse", "ID,Display_lang,ItemLevel,RequiredLevel,OverallQualityID,InventoryType,AllowableClass,Bonding,StatModifier_bonusStat_0,StatPercentEditor_0\n", `ItemSparse: required column "ItemDelay" missing`},
		{"stat percent missing", "ItemSparse", "ID,Display_lang,ItemLevel,RequiredLevel,OverallQualityID,InventoryType,AllowableClass,Bonding,StatModifier_bonusStat_0,ItemDelay\n", `ItemSparse: required column "StatPercentEditor_0" missing`},
		{"sell price missing", "ItemSparse", "ID,Display_lang,ItemLevel,RequiredLevel,OverallQualityID,InventoryType,AllowableClass,Bonding,StatModifier_bonusStat_0,StatPercentEditor_0,ItemDelay,BuyPrice\n", `ItemSparse: required column "SellPrice" missing`},
		{"item set slot missing", "ItemSet", "ID,Name_lang,ItemID_0\n", `ItemSet: required column "ItemID_16" missing`},
		{"quest line order missing", "QuestLineXQuest", "ID,QuestID,QuestLineID\n", `QuestLineXQuest: required column "OrderIndex" missing`},
		{"quest line name missing", "QuestLine", "ID\n", `QuestLine: required column "Name_lang" missing`},
		{"uimap name missing", "UiMap", "ID,Flags\n", `UiMap: required column "Name_lang" missing`},
		{"trivial rank missing", "SkillLineAbility", "SkillLine,Spell,MinSkillLineRank,TrivialSkillLineRankHigh\n", `SkillLineAbility: required column "TrivialSkillLineRankLow" missing`},
		{"reagent count missing", "SpellReagents", "SpellID,Reagent_0,ReagentCount_0,Reagent_7\n", `SpellReagents: required column "ReagentCount_7" missing`},
		{"lock skill missing", "Lock", "ID,_Index_0,_Index_7,Skill_0,Type_0,Type_7\n", `Lock: required column "Skill_7" missing`},
		{"game object lock prop missing", "GameObjects", "ID,Name_lang,TypeID\n", `GameObjects: required column "PropValue_0" missing`},
		{"lock type name missing", "LockType", "ID\n", `LockType: required column "Name_lang" missing`},
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

func recipeFixture() RecipeInput {
	return RecipeInput{
		SkillLines: []Row{
			{"ID": "171", "DisplayName_lang": "Alchemy", "CategoryID": "11"},
			{"ID": "185", "DisplayName_lang": "Cooking", "CategoryID": "9"},
			{"ID": "2933", "DisplayName_lang": "Test Profession [DNT]", "CategoryID": "11"},
			{"ID": "762", "DisplayName_lang": "Riding", "CategoryID": "7"},
		},
		Abilities: []Row{
			{"SkillLine": "185", "Spell": "20", "MinSkillLineRank": "1", "TrivialSkillLineRankHigh": "60", "TrivialSkillLineRankLow": "45"},
			{"SkillLine": "171", "Spell": "10", "MinSkillLineRank": "1", "TrivialSkillLineRankHigh": "55", "TrivialSkillLineRankLow": "30"},
			{"SkillLine": "171", "Spell": "10", "MinSkillLineRank": "1"},
			{"SkillLine": "171", "Spell": "11", "MinSkillLineRank": "0", "TrivialSkillLineRankHigh": "0", "TrivialSkillLineRankLow": "0"},
			{"SkillLine": "171", "Spell": "12"},
			{"SkillLine": "2933", "Spell": "10"},
			{"SkillLine": "762", "Spell": "10"},
		},
		Reagents: []Row{
			{"SpellID": "10", "Reagent_0": "100", "ReagentCount_0": "2", "Reagent_1": "101", "ReagentCount_1": "1", "Reagent_2": "100", "ReagentCount_2": "3", "Reagent_3": "0", "ReagentCount_3": "0"},
			{"SpellID": "11", "Reagent_7": "102", "ReagentCount_7": "4"},
			{"SpellID": "12", "Reagent_0": "103", "ReagentCount_0": "0"},
		},
		Effects: []Row{
			{"SpellID": "10", "EffectIndex": "1", "Effect": "24", "EffectItemType": "900"},
			{"SpellID": "10", "EffectIndex": "0", "Effect": "24", "EffectItemType": "901"},
			{"SpellID": "20", "EffectIndex": "0", "Effect": "157", "EffectItemType": "902"},
			{"SpellID": "11", "EffectIndex": "0", "Effect": "6", "EffectItemType": "903"},
		},
		SpellNames: []Row{{"ID": "10", "Name_lang": "Elixir"}, {"ID": "20", "Name_lang": ""}},
	}
}

func TestBuildRecipes(t *testing.T) {
	in := recipeFixture()
	got := BuildRecipes(in)
	if len(got) != 3 || got[0].SkillLineID != 171 || got[0].SpellID != 10 || got[1].SpellID != 11 || got[2].SkillLineID != 185 {
		t.Fatalf("rows (profession lines only, reagents or created item, deduped, sorted): %+v", got)
	}
	r := got[0]
	if *r.Name != "Elixir" || r.SkillLineName != "Alchemy" || *r.CreatesItemID != 901 || *r.LearnSkill != 1 || *r.YellowSkill != 30 || *r.GreySkill != 55 {
		t.Errorf("recipe 10 (first effect index makes the item): %+v", r)
	}
	if got[1].CreatesItemID != nil || got[1].Name != nil || got[1].LearnSkill != nil || got[1].YellowSkill != nil || got[1].GreySkill != nil {
		t.Errorf("recipe 11 (reagents only, zero thresholds are null): %+v", got[1])
	}
	if got[2].Name != nil || *got[2].CreatesItemID != 902 {
		t.Errorf("recipe 20 (random item effect, no reagents): %+v", got[2])
	}
	reagents := BuildRecipeReagents(got, in)
	want := []RecipeReagent{{10, 100, 5}, {10, 101, 1}, {11, 102, 4}}
	if !reflect.DeepEqual(reagents, want) {
		t.Errorf("reagents (merged, zero counts dropped): %+v", reagents)
	}
}

func TestBuildCraftedSources(t *testing.T) {
	item := func(v int32) *int32 { return &v }
	skill := int16(75)
	recipes := []Recipe{
		{SpellID: 10, SkillLineID: 171, SkillLineName: "Alchemy", CreatesItemID: item(901), LearnSkill: &skill},
		{SpellID: 11, SkillLineID: 171, SkillLineName: "Alchemy"},
		{SpellID: 12, SkillLineID: 171, SkillLineName: "Alchemy", CreatesItemID: item(0)},
		{SpellID: 10, SkillLineID: 185, SkillLineName: "Cooking", CreatesItemID: item(901)},
		{SpellID: 20, SkillLineID: 185, SkillLineName: "Cooking", CreatesItemID: item(902)},
	}
	got := BuildCraftedSources(recipes)
	if len(got) != 2 {
		t.Fatalf("rows (no item, zero item and repeated spell skipped): %+v", got)
	}
	r := got[0]
	if r.ItemID != 901 || r.SourceKind != "crafted" || r.SourceID != 10 || *r.Profession != "Alchemy" || *r.LearnSkill != 75 ||
		r.Kills != 0 || r.Rate != 0 || r.RateLow != nil || r.Tier != nil || r.EverlootDate != nil {
		t.Errorf("crafted row: %+v", r)
	}
	if got[1].LearnSkill != nil || *got[1].Profession != "Cooking" {
		t.Errorf("null learn skill kept null: %+v", got[1])
	}
}

func TestMergeLootSourcesKeepsExistingRows(t *testing.T) {
	d := "2026-11-20"
	lo := 0.1
	base := []everloot.LootSource{
		{ItemID: 5, SourceKind: everloot.KindCreature, SourceID: 8, Kills: 4, Drops: 1, Rate: 0.25, RateLow: &lo, Reporters: 3, EverlootDate: &d},
		{ItemID: 901, SourceKind: everloot.KindObject, SourceID: 2, Rate: 1},
	}
	prof := "Alchemy"
	crafted := []everloot.LootSource{{ItemID: 901, SourceKind: everloot.KindCrafted, SourceID: 10, Profession: &prof}}
	got := MergeLootSources(base, crafted)
	if len(got) != 3 || got[0] != base[0] || got[1].SourceKind != "crafted" || got[2] != base[1] {
		t.Fatalf("merge (existing rows unchanged, sorted by item then kind): %+v", got)
	}
	if len(base) != 2 {
		t.Errorf("base slice modified")
	}
}

func instanceFixture() InstanceInput {
	return InstanceInput{
		Maps: []Row{
			{"ID": "36", "MapName_lang": "Deadmines", "InstanceType": "1", "CorpseMapID": "0", "Corpse_0": "-11207.5", "Corpse_1": "1681", "MaxPlayers": "10"},
			{"ID": "409", "MapName_lang": "Molten Core", "InstanceType": "2", "CorpseMapID": "0", "Corpse_0": "-7510", "Corpse_1": "-1036", "MaxPlayers": "40"},
			{"ID": "2921", "MapName_lang": "Naxxramas", "InstanceType": "1", "CorpseMapID": "-1", "Corpse_0": "0", "Corpse_1": "0", "MaxPlayers": "0"},
			{"ID": "13", "MapName_lang": "Test Dungeon", "InstanceType": "1"},
			{"ID": "44", "MapName_lang": "<unused> Monastery", "InstanceType": "1"},
			{"ID": "0", "MapName_lang": "Eastern Kingdoms", "InstanceType": "0"},
		},
		MapDifficulties: []Row{
			{"ID": "9", "MapID": "36", "ContentTuningID": "2"},
			{"ID": "8", "MapID": "36", "ContentTuningID": "1"},
			{"ID": "7", "MapID": "409", "ContentTuningID": "1"},
		},
		ContentTuning: []Row{
			{"ID": "1", "LfgMinLevel": "0", "LfgMaxLevel": "0"},
			{"ID": "2", "LfgMinLevel": "13", "LfgMaxLevel": "18"},
		},
		Encounters: []Row{
			{"ID": "2", "MapID": "36", "Name_lang": "Cookie", "OrderIndex": "1000", "DifficultyID": "0"},
			{"ID": "1", "MapID": "36", "Name_lang": "Rhahk'Zor", "OrderIndex": "0", "DifficultyID": "0"},
			{"ID": "3", "MapID": "409", "Name_lang": "Ragnaros", "OrderIndex": "0", "DifficultyID": "0"},
			{"ID": "4", "MapID": "0", "Name_lang": "Azuregos", "OrderIndex": "0", "DifficultyID": "0"},
		},
		Creatures: []Row{
			{"ID": "100", "Name_lang": "Cookie"}, {"ID": "101", "Name_lang": "Rhahk'Zor"}, {"ID": "102", "Name_lang": "Rhahk'Zor"},
			{"ID": "103", "Name_lang": "Ragnaros"},
		},
	}
}

func TestBuildInstances(t *testing.T) {
	in := instanceFixture()
	got := BuildInstances(in)
	if len(got) != 3 || got[0].MapID != 36 || got[1].MapID != 409 || got[2].MapID != 2921 {
		t.Fatalf("rows (dungeon and raid maps, no test or unused maps, sorted): %+v", got)
	}
	d := got[0]
	if d.InstanceType != "dungeon" || got[1].InstanceType != "raid" || *d.MaxPlayers != 10 || *d.EntranceMapID != 0 || *d.EntranceX != -11207.5 || *d.EntranceY != 1681 {
		t.Errorf("deadmines: %+v", d)
	}
	if *d.MinLevel != 13 || *d.MaxLevel != 18 {
		t.Errorf("level from the lowest-id difficulty with a non-zero tuning: %+v", d)
	}
	if got[1].MinLevel != nil || got[1].MaxLevel != nil {
		t.Errorf("zero tuning levels are null: %+v", got[1])
	}
	if n := got[2]; n.EntranceMapID != nil || n.EntranceX != nil || n.MaxPlayers != nil {
		t.Errorf("no entrance and no players are null: %+v", n)
	}
	bosses := BuildInstanceBosses(got, in)
	if len(bosses) != 3 || bosses[0].Name != "Rhahk'Zor" || bosses[1].Name != "Cookie" || bosses[2].MapID != 409 {
		t.Fatalf("bosses (instance maps only, ordered): %+v", bosses)
	}
	if bosses[0].CreatureID != nil || *bosses[1].CreatureID != 100 || *bosses[2].CreatureID != 103 {
		t.Errorf("creature id only on a unique exact name: %+v", bosses)
	}
}

func TestApplyNpcZones(t *testing.T) {
	npcs := []Npc{{ID: 1}, {ID: 2}, {ID: 3}}
	uiMaps := []Row{{"ID": "1413", "Name_lang": "The Barrens"}, {"ID": "9", "Name_lang": ""}}
	ApplyNpcZones(npcs, uiMaps, map[int32]everloot.NpcZone{1: {ZoneID: 1413}, 2: {ZoneID: 9}})
	if z := npcs[0]; z.ZoneID == nil || *z.ZoneID != 1413 || z.ZoneName == nil || *z.ZoneName != "The Barrens" || z.ZoneSource == nil || *z.ZoneSource != ZoneSourceEverloot {
		t.Errorf("named zone: %+v", z)
	}
	if z := npcs[1]; z.ZoneID == nil || *z.ZoneID != 9 || z.ZoneName != nil || z.ZoneSource == nil {
		t.Errorf("unnamed UiMap keeps the id and no name: %+v", z)
	}
	if z := npcs[2]; z.ZoneID != nil || z.ZoneName != nil || z.ZoneSource != nil {
		t.Errorf("no source, no zone: %+v", z)
	}
	loot := []everloot.LootSource{{SourceKind: everloot.KindCreature, SourceID: 1}, {SourceKind: everloot.KindCreature, SourceID: 3}, {SourceKind: everloot.KindCreature, SourceID: 99}, {SourceKind: everloot.KindObject, SourceID: 1}}
	if c, in, z := ZoneCoverage(npcs, loot); c != 3 || in != 2 || z != 1 {
		t.Errorf("coverage %d %d %d", c, in, z)
	}
}

func TestBuildItemSets(t *testing.T) {
	got := BuildItemSets([]Row{
		{"ID": "41", "Name_lang": "B Set", "ItemID_0": "9", "ItemID_1": "0", "ItemID_2": "9"},
		{"ID": "1", "Name_lang": "A Set", "ItemID_0": "30", "ItemID_1": "20", "ItemID_16": "10"},
		{"ID": "2", "Name_lang": "Empty"},
	})
	want := []ItemSetRow{{1, "A Set", 10}, {1, "A Set", 20}, {1, "A Set", 30}, {41, "B Set", 9}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestBuildQuestLines(t *testing.T) {
	got := BuildQuestLines(
		[]Row{{"ID": "6", "Name_lang": "Soil"}, {"ID": "7", "Name_lang": ""}},
		[]Row{
			{"QuestLineID": "6", "QuestID": "92745", "OrderIndex": "1"},
			{"QuestLineID": "6", "QuestID": "92744", "OrderIndex": "0"},
			{"QuestLineID": "7", "QuestID": "5", "OrderIndex": "0"},
		})
	if len(got) != 3 || got[0].QuestID != 92744 || got[1].QuestID != 92745 || got[1].Position != 1 {
		t.Fatalf("order: %+v", got)
	}
	if got[0].Name == nil || *got[0].Name != "Soil" || got[2].Name != nil {
		t.Errorf("names: %+v", got)
	}
}
