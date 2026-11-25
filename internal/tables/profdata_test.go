package tables

import (
	"os"
	"testing"

	"github.com/WoW-Companion/data/internal/everloot"
)

func rn(s string) *string { return &s }

func recipeFx() []Recipe {
	return []Recipe{
		{SpellID: 100, SkillLineID: 1, SkillLineName: "Alchemy", Name: rn("Elixir")},
		{SpellID: 101, SkillLineID: 1, SkillLineName: "Alchemy", Name: rn("Potion")},
		{SpellID: 102, SkillLineID: 1, SkillLineName: "Alchemy", Name: rn("Potion")}, // same name twice: no link
		{SpellID: 200, SkillLineID: 2, SkillLineName: "Cooking", Name: rn("Roast")},
	}
}

func TestBuildRecipeItemsAndLearnedFrom(t *testing.T) {
	in := RecipeItemInput{
		ItemEffects: []Row{
			{"ID": "1", "SpellID": "5000", "TriggerType": "0"},
			{"ID": "2", "SpellID": "5001", "TriggerType": "0"},
			{"ID": "3", "SpellID": "5000", "TriggerType": "1"}, // not on use
			{"ID": "4", "SpellID": "5002", "TriggerType": "0"}, // teaches a non-recipe
			{"ID": "5", "SpellID": "101", "TriggerType": "6"},  // learn-spell form: SpellID is the recipe
			{"ID": "6", "SpellID": "999", "TriggerType": "6"},  // learns a non-recipe
		},
		ItemLinks: []Row{{"ItemEffectID": "1", "ItemID": "9001"}, {"ItemEffectID": "2", "ItemID": "9002"}, {"ItemEffectID": "3", "ItemID": "9003"}, {"ItemEffectID": "4", "ItemID": "9004"}, {"ItemEffectID": "5", "ItemID": "9005"}, {"ItemEffectID": "6", "ItemID": "9006"}},
		SpellEffects: []Row{
			{"SpellID": "5000", "Effect": "36", "EffectTriggerSpell": "100"},
			{"SpellID": "5001", "Effect": "36", "EffectTriggerSpell": "200"},
			{"SpellID": "5002", "Effect": "36", "EffectTriggerSpell": "999"},
			{"SpellID": "5003", "Effect": "24", "EffectTriggerSpell": "101"}, // not LearnSpell
		},
		Items: []Row{{"ID": "9001", "RequiredSkillRank": "125"}, {"ID": "9002", "RequiredSkillRank": "0"}},
	}
	items := BuildRecipeItems(recipeFx(), in)
	if len(items) != 3 || items[2].ItemID != 9005 || items[2].TeachesSpellID != 101 {
		t.Fatalf("learn-spell form: %+v", items)
	}
	if items[0].ItemID != 9001 || items[0].TeachesSpellID != 100 || items[0].Profession != "Alchemy" ||
		*items[0].RequiredSkill != 125 || items[1].Profession != "Cooking" || items[1].RequiredSkill != nil {
		t.Fatalf("items %+v", items)
	}
	rs := recipeFx()
	tr, it, both := ApplyLearnedFrom(rs, items[:2], map[int32]bool{100: true, 101: true})
	got := []string{rs[0].LearnedFrom, rs[1].LearnedFrom, rs[2].LearnedFrom, rs[3].LearnedFrom}
	want := []string{LearnedBoth, LearnedTrainer, LearnedTrainer, LearnedItem}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("recipe %d: %s want %s", i, got[i], want[i])
		}
	}
	if tr != 2 || it != 1 || both != 1 {
		t.Errorf("counts %d %d %d", tr, it, both)
	}
}

func TestBuildTrainers(t *testing.T) {
	offers := []everloot.TrainerOffer{
		{NpcID: 7, Profession: "Alchemy", Recipe: "Elixir", CostCopper: func() *int64 { v := int64(300); return &v }(), Reporters: 3},
		{NpcID: 7, Profession: "Alchemy", Recipe: "Potion", Reporters: 2}, // ambiguous name: spell null
		{NpcID: 8, Profession: "Cooking", Recipe: "Roast", Reporters: 2},
	}
	npcs := []Npc{{ID: 7, Name: "Trainer Bo"}}
	zones := map[int32]everloot.NpcZone{7: {ZoneID: 1413}}
	ui := []Row{{"ID": "1413", "Name_lang": "The Barrens"}}
	tr, ts := BuildTrainers(offers, recipeFx(), npcs, zones, ui, nil)
	if len(tr) != 2 || *tr[0].Name != "Trainer Bo" || *tr[0].ZoneName != "The Barrens" || tr[0].Recipes != 2 || tr[0].Reporters != 3 || tr[1].Name != nil || tr[1].ZoneID != nil {
		t.Fatalf("trainers %+v", tr)
	}
	if *ts[0].SpellID != 100 || ts[1].SpellID != nil || *ts[2].SpellID != 200 {
		t.Errorf("spells %+v", ts)
	}
	if got := TrainedSpells(ts); !got[100] || got[101] || !got[200] {
		t.Errorf("trained %v", got)
	}
}

func TestBuildGathering(t *testing.T) {
	obs := []everloot.NodeObs{
		{ObjectID: 1731, ZoneID: 1413, Name: "Copper Vein", GatherKind: "ore", Windows: 3, Reporters: 2},
		{ObjectID: 1618, ZoneID: 1413, Name: "Peacebloom", GatherKind: "herb", Windows: 2, Reporters: 2},
		{ObjectID: 178204, ZoneID: 1434, Name: "Axe Shipment", GatherKind: "chest", Windows: 2, Reporters: 2},
		{ObjectID: 2040, ZoneID: 1434, Name: "Odd", GatherKind: "other", Windows: 2, Reporters: 2},
	}
	yields := []everloot.NodeYield{{ObjectID: 1731, ItemID: 2770}, {ObjectID: 178204, ItemID: 16745}}
	nodes := map[int32]Node{2040: {ID: 2040, Name: "Tin Vein", Profession: "Mining", SkillRequired: 65}}
	ui := []Row{{"ID": "1413", "Name_lang": "The Barrens"}}
	rows, ys := BuildGathering(obs, yields, nodes, ui, nil)
	if len(rows) != 3 {
		t.Fatalf("rows %+v", rows)
	}
	if rows[0].Profession != "Mining" || rows[0].SkillRequired == nil || *rows[0].SkillRequired != 1 || *rows[0].ZoneName != "The Barrens" || rows[1].Profession != "Herbalism" {
		t.Errorf("rows %+v", rows)
	}
	if rows[2].Profession != "Mining" || *rows[2].SkillRequired != 65 || *rows[2].Name != "Tin Vein" { // the client's lock wins
		t.Errorf("client node %+v", rows[2])
	}
	if len(ys) != 1 || ys[0].NodeID != 1731 {
		t.Errorf("yields %+v", ys)
	}
}

func TestBuildFishingCatches(t *testing.T) {
	got := BuildFishingCatches([]everloot.FishCatch{{ZoneID: 1413, ItemID: 6291, Casts: 5, Catches: 3, Reporters: 2, Rate: 0.6}}, []Row{{"ID": "1413", "Name_lang": "The Barrens"}})
	if len(got) != 1 || *got[0].ZoneName != "The Barrens" || got[0].Rate != 0.6 {
		t.Errorf("%+v", got)
	}
}

func TestParseCuratedProfessions(t *testing.T) {
	ok := `status: "advisor default, not authoritative"
professions:
  - {name: Mining, kind: primary, text: "Ore, and bars."}
ranks:
  - {rank: Apprentice, skill_min: 1, skill_cap: 75, min_level: 5, text: "First."}
pairings:
  - {a: Mining, b: Blacksmithing, text: "Bars."}
rules:
  - {topic: skinning, text: "About five times the level, from level 10 up."}
`
	c, err := ParseCuratedProfessions("f", ok)
	if err != nil || len(c.Rows) != 4 || c.Rows[0].Section != "pairings" || *c.Rows[1].Related != "primary" || *c.Rows[2].SkillCap != 75 || *c.Rows[2].MinLevel != 5 {
		t.Fatalf("%+v %v", c.Rows, err)
	}
	if err := c.Check([]string{"Mining", "Blacksmithing"}); err != nil {
		t.Error(err)
	}
	if err := c.Check([]string{"Mining"}); err == nil {
		t.Error("an unknown profession name must fail")
	}
	if err := (CuratedProfessions{Status: "authoritative"}).Check(nil); err == nil {
		t.Error("status must say advisor default")
	}
	for name, bad := range map[string]string{
		"unknown section": "weird:\n",
		"unknown field":   "professions:\n  - {name: A, kind: primary, text: x, extra: y}\n",
		"missing field":   "professions:\n  - {name: A, kind: primary}\n",
		"bad kind":        "professions:\n  - {name: A, kind: third, text: x}\n",
		"bad number":      "ranks:\n  - {rank: A, skill_min: x, skill_cap: 75, min_level: 5, text: t}\n",
		"inverted range":  "ranks:\n  - {rank: A, skill_min: 75, skill_cap: 75, min_level: 5, text: t}\n",
		"row off section": "  - {name: A}\n",
	} {
		if _, err := ParseCuratedProfessions("f", bad); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
}

func TestCuratedProfessionsFileParses(t *testing.T) {
	c, err := LoadCuratedProfessions("../../curated/professions.yaml", nil)
	if err != nil {
		t.Fatal(err)
	}
	primary, secondary := 0, 0
	for _, r := range c.Rows {
		if r.Section == "professions" {
			if *r.Related == "primary" {
				primary++
			} else {
				secondary++
			}
		}
	}
	if primary != 9 || secondary != 3 {
		t.Errorf("primary %d secondary %d", primary, secondary)
	}
	if _, err := os.Stat("../../curated/professions.yaml"); err != nil {
		t.Fatal(err)
	}
}

func TestBuildGatheringCuratedSkillAndDropYields(t *testing.T) {
	obs := []everloot.NodeObs{
		{ObjectID: 1734, ZoneID: 1413, Name: "Gold Vein", GatherKind: "other", Windows: 2, Reporters: 2},
		{ObjectID: 2047, ZoneID: 1413, Name: "Truesilver Deposit", GatherKind: "ore", Windows: 2, Reporters: 2},
		{ObjectID: 5, ZoneID: 1413, Name: "Mystery Vein", GatherKind: "ore", Windows: 2, Reporters: 2},
		{ObjectID: 6, ZoneID: 1413, Name: "Peacebloom", GatherKind: "chest", Windows: 2, Reporters: 2},
	}
	yields := []everloot.NodeYield{{ObjectID: 2047, ItemID: 7911, Windows: 4, Reporters: 3, MinCount: 1, MaxCount: 2}}
	drops := []everloot.LootSource{
		{ItemID: 2776, SourceKind: everloot.KindObject, SourceID: 1734, Kills: 40, Drops: 40, Reporters: 3},
		{ItemID: 7911, SourceKind: everloot.KindObject, SourceID: 2047, Kills: 9, Drops: 9, Reporters: 2}, // already a window yield: kept as it is
		{ItemID: 7910, SourceKind: everloot.KindGathered, SourceID: 2047, Kills: 5, Drops: 5, Reporters: 2},
		{ItemID: 1, SourceKind: everloot.KindCreature, SourceID: 2047, Kills: 5, Drops: 5, Reporters: 2}, // not a node source
		{ItemID: 2, SourceKind: everloot.KindObject, SourceID: 999, Kills: 5, Drops: 5, Reporters: 2},    // not a kept node
		{ItemID: 3, SourceKind: everloot.KindObject, SourceID: 1734, Kills: 5, Drops: 0, Reporters: 2},   // never dropped
	}
	rows, ys := BuildGathering(obs, yields, nil, nil, drops)
	if len(rows) != 4 {
		t.Fatalf("rows %+v", rows)
	}
	want := map[int32]struct {
		prof  string
		skill *int16
	}{1734: {"Mining", i16(155)}, 2047: {"Mining", i16(230)}, 5: {"Mining", nil}, 6: {"Herbalism", i16(1)}}
	for _, r := range rows {
		w := want[r.NodeID]
		if r.Profession != w.prof || (r.SkillRequired == nil) != (w.skill == nil) || (w.skill != nil && *r.SkillRequired != *w.skill) {
			t.Errorf("node %d: %s %v", r.NodeID, r.Profession, r.SkillRequired)
		}
	}
	got := map[[2]int32]GatheringYield{}
	for _, y := range ys {
		got[[2]int32{y.NodeID, y.ItemID}] = y
	}
	if len(ys) != 3 {
		t.Fatalf("yields %+v", ys)
	}
	if y := got[[2]int32{1734, 2776}]; y.Windows != 40 || y.MinCount != 0 || y.Reporters != 3 {
		t.Errorf("gold yield %+v", y)
	}
	if y := got[[2]int32{2047, 7911}]; y.MaxCount != 2 || y.Windows != 4 {
		t.Errorf("window yield replaced %+v", y)
	}
	if _, ok := got[[2]int32{2047, 7910}]; !ok {
		t.Error("gathered-kind drop row not used")
	}
}
