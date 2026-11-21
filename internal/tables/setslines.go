package tables

import (
	"slices"
	"strconv"
)

const setItemSlots = 17

// ItemSetRow is one member item of an item set, one row per set and item.
type ItemSetRow struct {
	SetID  int32  `parquet:"set_id"`
	Name   string `parquet:"name"`
	ItemID int32  `parquet:"item_id"`
}

// QuestLineRow is one quest of a quest line, in line order. Name is null when QuestLine
// has no named row for the line.
type QuestLineRow struct {
	LineID   int32   `parquet:"line_id"`
	Name     *string `parquet:"name,optional"`
	Position int32   `parquet:"position"`
	QuestID  int32   `parquet:"quest_id"`
}

// BuildItemSets lists the member items of every set (ItemSet.ItemID_0..16; 0 marks an empty
// slot), sorted by set id then item id. An item listed twice in a set is kept once.
func BuildItemSets(sets []Row) []ItemSetRow {
	out := []ItemSetRow{}
	for _, r := range sets {
		id := r.int("ID", 0)
		seen := map[int]bool{}
		for i := 0; i < setItemSlots; i++ {
			item := r.int("ItemID_"+strconv.Itoa(i), 0)
			if item == 0 || seen[item] {
				continue
			}
			seen[item] = true
			out = append(out, ItemSetRow{SetID: int32(id), Name: r["Name_lang"], ItemID: int32(item)})
		}
	}
	slices.SortStableFunc(out, func(a, b ItemSetRow) int {
		if a.SetID != b.SetID {
			return int(a.SetID) - int(b.SetID)
		}
		return int(a.ItemID) - int(b.ItemID)
	})
	return out
}

// BuildQuestLines lists the quests of every line in the order the client gives
// (QuestLineXQuest.OrderIndex), sorted by line id, position, then quest id.
func BuildQuestLines(lines, members []Row) []QuestLineRow {
	name := make(map[int]string, len(lines))
	for _, l := range lines {
		if n := l["Name_lang"]; n != "" {
			name[l.int("ID", 0)] = n
		}
	}
	out := make([]QuestLineRow, 0, len(members))
	for _, m := range members {
		id := m.int("QuestLineID", 0)
		row := QuestLineRow{LineID: int32(id), Position: int32(m.int("OrderIndex", 0)), QuestID: int32(m.int("QuestID", 0))}
		if n, ok := name[id]; ok {
			row.Name = &n
		}
		out = append(out, row)
	}
	slices.SortStableFunc(out, func(a, b QuestLineRow) int {
		if a.LineID != b.LineID {
			return int(a.LineID) - int(b.LineID)
		}
		if a.Position != b.Position {
			return int(a.Position) - int(b.Position)
		}
		return int(a.QuestID) - int(b.QuestID)
	})
	return out
}

// price is a copper amount from an ItemSparse column; null when the client stores zero
// (no price) or the value is not a number.
func price(r Row, key string) *int32 {
	if n := r.int(key, 0); n > 0 {
		return ptr(n)
	}
	return nil
}
