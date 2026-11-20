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
