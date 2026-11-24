package tables

import (
	"strconv"
	"strings"
	"testing"

	"github.com/WoW-Companion/data/internal/everloot"
)

const curatedFixture = `# comment
status: "advisor default, not authoritative; typical ranges"
instances:
  - {map: 36, name: "Deadmines", min: 15, max: 25, zone_id: 1436, zone: "Westfall", source: "test"}
  - {map: 389, name: "Ragefire Chasm", min: 13, max: 18, zone_id: 1454, zone: "Orgrimmar", source: "test"}
`

var zoneFixture = []Row{{"ID": "1436", "Name_lang": "Westfall"}, {"ID": "1454", "Name_lang": "Orgrimmar"}, {"ID": "1411", "Name_lang": "Durotar"}}

func fixtureInstances() []Instance {
	return []Instance{{MapID: 36, Name: "Deadmines"}, {MapID: 389, Name: "Ragefire Chasm"}, {MapID: 409, Name: "Molten Core"}}
}

func TestParseCuratedInstances(t *testing.T) {
	c, err := ParseCuratedInstances("f", curatedFixture)
	if err != nil || len(c.Rows) != 2 || c.Rows[1].Min != 13 || c.Rows[1].Zone != "Orgrimmar" || !strings.Contains(c.Status, "advisor default") {
		t.Fatalf("%+v %v", c, err)
	}
	for _, bad := range []string{
		"status: x\ninstances:\n  - {map: 1, name: A}\n",
		"status: x\ninstances:\n  - {map: x, name: A, min: 1, max: 2, zone_id: 1, zone: Z, source: s}\n",
		"status: x\ninstances:\n  - {map: 1, name: A, min: 1, max: 2, zone_id: 1, zone: Z, source: s, extra: 1}\n",
		"status: x\n  - {map: 1}\n",
	} {
		if _, err := ParseCuratedInstances("f", bad); err == nil {
			t.Errorf("accepted %q", bad)
		}
	}
}

// Client levels beat curated ones; a corroborated observed zone beats the curated zone.
func TestApplyInstanceDataPrecedence(t *testing.T) {
	cur, _ := ParseCuratedInstances("f", curatedFixture)
	in := fixtureInstances()
	in[0].MinLevel, in[0].MaxLevel, in[0].LevelSource = to(int16(10)), to(int16(12)), to(LevelClient)
	obs := map[int32]everloot.EntranceZone{389: {UIMapID: 1411, Reporters: 2}}
	if err := ApplyInstanceData(in, cur, zoneFixture, obs); err != nil {
		t.Fatal(err)
	}
	if *in[0].MinLevel != 10 || *in[0].MaxLevel != 12 || *in[0].LevelSource != LevelClient {
		t.Errorf("client level kept: %+v", in[0])
	}
	if *in[0].EntranceZoneID != 1436 || *in[0].EntranceZoneName != "Westfall" || *in[0].EntranceZoneSource != ZoneCurated {
		t.Errorf("curated zone fills: %+v", in[0])
	}
	r := in[1]
	if *r.MinLevel != 13 || *r.MaxLevel != 18 || *r.LevelSource != LevelCurated {
		t.Errorf("curated level fills: %+v", r)
	}
	if *r.EntranceZoneID != 1411 || *r.EntranceZoneName != "Durotar" || *r.EntranceZoneSource != ZoneObserved {
		t.Errorf("observed zone wins: %+v", r)
	}
	if m := in[2]; m.MinLevel != nil || m.LevelSource != nil || m.EntranceZoneID != nil {
		t.Errorf("an instance with no data stays null: %+v", m)
	}
}

func TestApplyInstanceDataRejectsBadCuratedRows(t *testing.T) {
	for name, text := range map[string]string{
		"unknown map":   strings.Replace(curatedFixture, "map: 36", "map: 99", 1),
		"wrong name":    strings.Replace(curatedFixture, `"Deadmines"`, `"Deadmine"`, 1),
		"wrong zone":    strings.Replace(curatedFixture, `"Westfall"`, `"Duskwood"`, 1),
		"reversed":      strings.Replace(curatedFixture, "min: 15, max: 25", "min: 30, max: 25", 1),
		"no advisor":    strings.Replace(curatedFixture, "advisor default", "authoritative", 1),
		"duplicate row": curatedFixture + `  - {map: 36, name: "Deadmines", min: 15, max: 25, zone_id: 1436, zone: "Westfall", source: "t"}` + "\n",
	} {
		cur, err := ParseCuratedInstances("f", text)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if err := ApplyInstanceData(fixtureInstances(), cur, zoneFixture, nil); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestLinkBossesPrefersTheLootSource(t *testing.T) {
	bosses := []InstanceBoss{{Name: "Smite"}, {Name: "Twin"}, {Name: "Lost"}}
	npcs := []Npc{{ID: 1, Name: "Smite"}, {ID: 2, Name: "Smite"}, {ID: 3, Name: "Twin"}, {ID: 4, Name: "Twin"}, {ID: 5, Name: "Lost"}, {ID: 6, Name: "Lost"}}
	n := LinkBosses(bosses, npcs, map[int32]bool{2: true, 3: true, 4: true})
	if n != 1 || bosses[0].CreatureID == nil || *bosses[0].CreatureID != 2 || bosses[1].CreatureID != nil || bosses[2].CreatureID != nil {
		t.Errorf("one candidate with loot links; two or none do not: %d %+v", n, bosses)
	}
}

func TestApplyEncounterObsAndTagInstanceLoot(t *testing.T) {
	bosses := []InstanceBoss{
		{MapID: 36, EncounterID: 1, CreatureID: to(int32(100))},
		{MapID: 36, EncounterID: 2},
		{MapID: 389, EncounterID: 3, CreatureID: to(int32(200))},
		{MapID: 409, EncounterID: 4, CreatureID: to(int32(200))}, // the same creature in two maps: untagged
	}
	if n := ApplyEncounterObs(bosses, map[everloot.EncounterKey]int32{{MapID: 36, EncounterID: 1}: 3, {MapID: 36, EncounterID: 9}: 2}); n != 1 || *bosses[0].Reporters != 3 || bosses[1].Reporters != nil {
		t.Errorf("corroboration: %d %+v", n, bosses)
	}
	rows := []everloot.LootSource{
		{ItemID: 1, SourceKind: everloot.KindCreature, SourceID: 100},
		{ItemID: 2, SourceKind: everloot.KindCreature, SourceID: 101},
		{ItemID: 3, SourceKind: everloot.KindObject, SourceID: 100}, // an object, not the creature
		{ItemID: 4, SourceKind: everloot.KindCreature, SourceID: 200},
	}
	if n := TagInstanceLoot(rows, bosses); n != 1 || rows[0].MapID == nil || *rows[0].MapID != 36 || rows[1].MapID != nil || rows[2].MapID != nil || rows[3].MapID != nil {
		t.Errorf("tagging: %d %+v", n, rows)
	}
}

func TestBuildInstanceQuests(t *testing.T) {
	ins := []Instance{
		{MapID: 36, Name: "Deadmines"}, {MapID: 533, Name: "Naxxramas", LevelSource: to(LevelCurated)}, {MapID: 2921, Name: "Naxxramas"},
		{MapID: 1, Name: "Twin"}, {MapID: 2, Name: "Twin"},
	}
	obs := []everloot.QuestObserved{
		{QuestID: 9, Category: to("The Deadmines")}, {QuestID: 8, Category: to("deadmines")}, {QuestID: 7, Category: to("Elwynn Forest")},
		{QuestID: 6}, {QuestID: 5, Category: to("Naxxramas")}, {QuestID: 4, Category: to("Twin")},
	}
	cl := []Quest{{ID: 8, ZoneName: to("Deadmines")}, {ID: 3, ZoneName: to("Westfall")}, {ID: 2}}
	got := BuildInstanceQuests(ins, obs, cl)
	var s []string
	for _, r := range got {
		s = append(s, strings.Join([]string{itoa(r.MapID), itoa(r.QuestID), r.Reason}, "/"))
	}
	if want := "36/8/category 36/8/zone 36/9/category 533/5/category"; strings.Join(s, " ") != want {
		t.Errorf("got %v want %s", s, want)
	}
}

func itoa(n int32) string { return strconv.Itoa(int(n)) }
