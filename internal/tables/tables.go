// Package tables builds the `items` and `quests` tables from cached client CSVs.
//
// Only facts are kept: ids, numbers, flags, slots and names. Long-form text columns
// (item descriptions, quest text) are never read into the output.
package tables

import (
	"encoding/csv"
	"fmt"
	"io"
	"slices"
	"strconv"
)

const statSlots = 10

// Stats status values for Item.StatsStatus.
const (
	StatsKnown   = "known"
	StatsNone    = "none"
	StatsUnknown = "unknown"
)

// Gear classes carry stats when equippable: weapons (2) and armor (4); quality 2
// (uncommon) and up is expected to carry stats.
const minStattedQuality = 2

func isGearClass(c int) bool { return c == 2 || c == 4 }

// Item is one row of the items table.
type Item struct {
	ID                 int32      `parquet:"id"`
	Name               string     `parquet:"name"`
	ItemLevel          int16      `parquet:"item_level"`
	RequiredLevel      int16      `parquet:"required_level"`
	Quality            int8       `parquet:"quality"`
	ClassID            int8       `parquet:"class_id"`
	SubclassID         int8       `parquet:"subclass_id"`
	Slot               int8       `parquet:"slot"`
	AllowableClassMask int32      `parquet:"allowable_class_mask"`
	Bonding            int8       `parquet:"bonding"`
	StatTypes          []int16    `parquet:"stat_types,list"`
	StatsStatus        string     `parquet:"stats_status"`
	Stats              []ItemStat `parquet:"stats,list"`
	Armor              *int32     `parquet:"armor,optional"`
	DmgMin             *int32     `parquet:"dmg_min,optional"`
	DmgMax             *int32     `parquet:"dmg_max,optional"`
	SpeedMS            *int32     `parquet:"speed_ms,optional"`
}

// Armor, DmgMin and DmgMax stay null: this build's client tables hold no armor or damage
// amount for an item, only per-level budget tables (docs/sources.md), and nothing is inferred.

// Quest is one row of the quests table. The client's quest tables carry no name,
// level or rewards; only what exists is kept. Level and MinLevel stay null: no allowed
// source holds them (docs/sources.md). The zone is the UiMap of the quest's first POI.
type Quest struct {
	ID            int32   `parquet:"id"`
	UniqueBitFlag int32   `parquet:"unique_bit_flag"`
	UIThemeID     int32   `parquet:"ui_theme_id"`
	QuestLineID   *int32  `parquet:"quest_line_id,optional"`
	MapID         *int32  `parquet:"map_id,optional"`
	UIMapID       *int32  `parquet:"ui_map_id,optional"`
	Level         *int16  `parquet:"level,optional"`
	MinLevel      *int16  `parquet:"min_level,optional"`
	ZoneID        *int32  `parquet:"zone_id,optional"`
	ZoneName      *string `parquet:"zone_name,optional"`
}

// Row is one CSV record keyed by header name.
type Row map[string]string

// Required lists the columns each client table must carry in its header. A column the
// export lacks is an error naming table and column, never a silent null or zero.
var Required = map[string][]string{
	"ItemSparse":         {"ID", "Display_lang", "ItemLevel", "RequiredLevel", "OverallQualityID", "InventoryType", "AllowableClass", "Bonding", "StatModifier_bonusStat_0", "StatPercentEditor_0", "ItemDelay"},
	"Item":               {"ID", "ClassID", "SubclassID"},
	"QuestV2":            {"ID", "UniqueBitFlag", "UiQuestDetailsThemeID"},
	"QuestLineXQuest":    {"ID", "QuestID", "QuestLineID"},
	"QuestPOIBlob":       {"ID", "QuestID", "MapID", "UiMapID"},
	"UiMap":              {"ID", "Name_lang"},
	"Creature":           {"ID", "Name_lang", "Classification", "CreatureType", "CreatureFamily"},
	"CreatureDifficulty": {"ID", "CreatureID", "MinLevel", "MaxLevel", "FactionTemplateID"},

	"TraitNode":                {"ID", "TraitTreeID"},
	"TraitNodeGroup":           {"ID", "TraitTreeID"},
	"TraitNodeGroupXTraitNode": {"TraitNodeGroupID", "TraitNodeID"},
	"TraitNodeXTraitNodeEntry": {"TraitNodeID", "TraitNodeEntryID", "_Index"},
	"TraitNodeEntry":           {"ID", "TraitDefinitionID", "MaxRanks"},
	"TraitDefinition":          {"ID", "SpellID", "OverrideName_lang"},
	"TraitEdge":                {"LeftTraitNodeID", "RightTraitNodeID"},
	"TraitCond":                {"TraitNodeGroupID", "SpentAmountRequired"},
	"SkillLineXTraitTree":      {"SkillLineID", "TraitTreeID"},
	"SkillRaceClassInfo":       {"SkillID", "ClassMask"},
	"ChrClasses":               {"ID", "Name_lang"},
	"SpellName":                {"ID", "Name_lang"},

	"SkillLine":        {"ID", "DisplayName_lang", "CategoryID"},
	"SkillLineAbility": {"SkillLine", "Spell", "MinSkillLineRank", "TrivialSkillLineRankHigh", "TrivialSkillLineRankLow"},
	"SpellReagents":    {"SpellID", "Reagent_0", "ReagentCount_0", "Reagent_7", "ReagentCount_7"},
	"SpellEffect":      {"SpellID", "EffectIndex", "Effect", "EffectItemType"},
}

// ReadTable parses a client CSV export and checks its header against Required[table].
func ReadTable(table string, r io.Reader) ([]Row, error) {
	rows, head, err := readCSV(r)
	if err != nil {
		return nil, err
	}
	for _, col := range Required[table] {
		if !slices.Contains(head, col) {
			return nil, fmt.Errorf("%s: required column %q missing from the header", table, col)
		}
	}
	return rows, nil
}

// ReadCSV parses a client CSV export into rows.
func ReadCSV(r io.Reader) ([]Row, error) {
	rows, _, err := readCSV(r)
	return rows, err
}

func readCSV(r io.Reader) ([]Row, []string, error) {
	cr := csv.NewReader(r)
	cr.FieldsPerRecord = -1
	head, err := cr.Read()
	if err == io.EOF {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	var rows []Row
	for {
		rec, err := cr.Read()
		if err == io.EOF {
			return rows, head, nil
		}
		if err != nil {
			return nil, nil, err
		}
		row := make(Row, len(head))
		for i, h := range head {
			if i < len(rec) {
				row[h] = rec[i]
			}
		}
		rows = append(rows, row)
	}
}

func (r Row) int(key string, def int) int {
	v := r[key]
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

// StatTypes returns the stat type ids the client lists for an item (-1 marks an empty slot).
func StatTypes(r Row) []int16 {
	out := []int16{}
	for i := 0; i < statSlots; i++ {
		if s := r.int("StatModifier_bonusStat_"+strconv.Itoa(i), -1); s != -1 {
			out = append(out, int16(s))
		}
	}
	return out
}

// StatsStatusOf is `known` when stats are listed; `unknown` when a stat-bearing piece
// lists none; `none` otherwise.
func StatsStatusOf(stats []int16, classID, slot, quality int) string {
	if len(stats) > 0 {
		return StatsKnown
	}
	if isGearClass(classID) && slot != 0 && quality >= minStattedQuality {
		return StatsUnknown
	}
	return StatsNone
}

// BuildItems joins ItemSparse to Item and returns rows sorted by id.
func BuildItems(sparse, items []Row) []Item {
	base := make(map[int]Row, len(items))
	for _, r := range items {
		base[r.int("ID", 0)] = r
	}
	out := make([]Item, 0, len(sparse))
	for _, r := range sparse {
		id := r.int("ID", 0)
		b := base[id]
		stats := StatTypes(r)
		classID := b.int("ClassID", 0)
		slot := r.int("InventoryType", 0)
		quality := r.int("OverallQualityID", 0)
		out = append(out, Item{
			ID:                 int32(id),
			Name:               r["Display_lang"],
			ItemLevel:          int16(r.int("ItemLevel", 0)),
			RequiredLevel:      int16(r.int("RequiredLevel", 0)),
			Quality:            int8(quality),
			ClassID:            int8(classID),
			SubclassID:         int8(b.int("SubclassID", 0)),
			Slot:               int8(slot),
			AllowableClassMask: int32(r.int("AllowableClass", -1)),
			Bonding:            int8(r.int("Bonding", 0)),
			StatTypes:          stats,
			StatsStatus:        StatsStatusOf(stats, classID, slot, quality),
			Stats:              ItemStats(r),
			SpeedMS:            SpeedMS(r, classID),
		})
	}
	slices.SortStableFunc(out, func(a, b Item) int { return int(a.ID) - int(b.ID) })
	return out
}

func firstBy(rows []Row, key string) map[int]Row {
	sorted := slices.Clone(rows)
	slices.SortStableFunc(sorted, func(a, b Row) int { return a.int("ID", 0) - b.int("ID", 0) })
	m := make(map[int]Row, len(rows))
	for _, r := range sorted {
		q := r.int(key, 0)
		if _, ok := m[q]; !ok {
			m[q] = r
		}
	}
	return m
}

func ptr(v int) *int32 { x := int32(v); return &x }

// BuildQuests returns quests with their first quest line, first map and zone, sorted by id.
// The zone is the first POI's UiMapID and that map's name; it is null when the POI has no
// UiMap (0) and the name is null when the UiMap table has no row for it.
func BuildQuests(quests, lines, pois, uiMaps []Row) []Quest {
	mapName := make(map[int]string, len(uiMaps))
	for _, m := range uiMaps {
		if n := m["Name_lang"]; n != "" {
			mapName[m.int("ID", 0)] = n
		}
	}
	lineOf := firstBy(lines, "QuestID")
	poiOf := firstBy(pois, "QuestID")
	out := make([]Quest, 0, len(quests))
	for _, r := range quests {
		id := r.int("ID", 0)
		q := Quest{
			ID:            int32(id),
			UniqueBitFlag: int32(r.int("UniqueBitFlag", 0)),
			UIThemeID:     int32(r.int("UiQuestDetailsThemeID", 0)),
		}
		if l, ok := lineOf[id]; ok {
			q.QuestLineID = ptr(l.int("QuestLineID", 0))
		}
		if p, ok := poiOf[id]; ok {
			q.MapID = ptr(p.int("MapID", 0))
			q.UIMapID = ptr(p.int("UiMapID", 0))
			if z := p.int("UiMapID", 0); z != 0 {
				q.ZoneID = ptr(z)
				if n, ok := mapName[z]; ok {
					q.ZoneName = &n
				}
			}
		}
		out = append(out, q)
	}
	slices.SortStableFunc(out, func(a, b Quest) int { return int(a.ID) - int(b.ID) })
	return out
}
