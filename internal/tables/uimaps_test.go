package tables

import "testing"

func TestBuildUiMaps(t *testing.T) {
	uiMaps := []Row{
		{"ID": "1458", "Name_lang": "Undercity", "ParentUiMapID": "1420", "Type": "3"},
		{"ID": "310", "Name_lang": "Shadowfang Keep", "ParentUiMapID": "217", "Type": "4"},
		{"ID": "279", "Name_lang": "Wailing Caverns", "ParentUiMapID": "0", "Type": "4"},
		{"ID": "9", "Name_lang": "", "ParentUiMapID": "0", "Type": "9"},
	}
	asg := []Row{
		{"ID": "5", "UiMapID": "310", "MapID": "0"}, // an open-world assignment first: skipped
		{"ID": "6", "UiMapID": "310", "MapID": "33"},
		{"ID": "7", "UiMapID": "279", "MapID": "43"},
		{"ID": "8", "UiMapID": "1458", "MapID": "0"},
	}
	got := BuildUiMaps(uiMaps, asg, map[int32]bool{33: true, 43: true})
	if len(got) != 4 || got[0].UIMapID != 9 {
		t.Fatalf("every row kept, sorted by id: %+v", got)
	}
	by := map[int32]UiMap{}
	for _, r := range got {
		by[r.UIMapID] = r
	}
	cases := []struct {
		id         int32
		name, typ  string
		parent, in int32 // 0 means null
	}{
		{310, "Shadowfang Keep", "dungeon", 217, 33},
		{279, "Wailing Caverns", "dungeon", 0, 43},
		{1458, "Undercity", "zone", 1420, 0},
		{9, "", "unknown", 0, 0},
	}
	for _, c := range cases {
		r := by[c.id]
		if r.Name != c.name || r.Type != c.typ {
			t.Errorf("%d: %+v", c.id, r)
		}
		if (c.parent == 0) != (r.ParentUIMap == nil) || (c.parent != 0 && *r.ParentUIMap != c.parent) {
			t.Errorf("%d parent: %v", c.id, r.ParentUIMap)
		}
		if (c.in == 0) != (r.InstanceMapID == nil) || (c.in != 0 && *r.InstanceMapID != c.in) {
			t.Errorf("%d instance map: %v", c.id, r.InstanceMapID)
		}
	}
}
