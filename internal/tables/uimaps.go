package tables

import "slices"

// UiMap is one row of ui_maps: a map the client's UI knows by a UiMap id (the ids that
// quest POIs, Questie and the addon use), whether a continent, a zone or a dungeon.
type UiMap struct {
	UIMapID int32  `parquet:"ui_map_id"`
	Name    string `parquet:"name"`
	// Type is the client's UiMap.Type enum as a label: see UiMapTypes.
	Type        string `parquet:"type"`
	TypeID      int16  `parquet:"type_id"`
	ParentUIMap *int32 `parquet:"parent_ui_map_id,optional"`
	// InstanceMapID is the instance map (Map.ID of a dungeon or raid, as in
	// instances.map_id) the UiMap is assigned to; null for an open-world map.
	InstanceMapID *int32 `parquet:"instance_map_id,optional"`
}

// UiMapTypes labels the client's UiMap.Type enum (Enum.UIMapType).
var UiMapTypes = map[int]string{0: "cosmic", 1: "world", 2: "continent", 3: "zone", 4: "dungeon", 5: "micro", 6: "orphan"}

// UiMapTypeLabel returns the label of a UiMap.Type value, "unknown" for one not in the enum.
func UiMapTypeLabel(t int) string {
	if l, ok := UiMapTypes[t]; ok {
		return l
	}
	return "unknown"
}

// BuildUiMaps makes one row per UiMap row, sorted by id, empty names kept (an unnamed map
// is reported by the caller, not dropped). A zero parent is stored as null. The instance
// map is the MapID of the UiMap's lowest-id UiMapAssignment whose MapID is in instanceMaps
// (the ids of instances.map_id); a UiMap with no such assignment gets null.
func BuildUiMaps(uiMaps, assignments []Row, instanceMaps map[int32]bool) []UiMap {
	asg := slices.Clone(assignments)
	slices.SortStableFunc(asg, func(a, b Row) int { return a.int("ID", 0) - b.int("ID", 0) })
	inst := map[int]int32{}
	for _, a := range asg {
		m := int32(a.int("MapID", 0))
		u := a.int("UiMapID", 0)
		if _, done := inst[u]; !done && instanceMaps[m] {
			inst[u] = m
		}
	}
	out := make([]UiMap, 0, len(uiMaps))
	for _, r := range uiMaps {
		id := r.int("ID", 0)
		t := r.int("Type", 0)
		row := UiMap{UIMapID: int32(id), Name: r["Name_lang"], Type: UiMapTypeLabel(t), TypeID: int16(t)}
		if p := r.int("ParentUiMapID", 0); p > 0 {
			row.ParentUIMap = ptr(p)
		}
		if m, ok := inst[id]; ok {
			row.InstanceMapID = &m
		}
		out = append(out, row)
	}
	slices.SortFunc(out, func(a, b UiMap) int { return int(a.UIMapID - b.UIMapID) })
	return out
}
