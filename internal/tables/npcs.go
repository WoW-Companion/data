package tables

import (
	"slices"
	"strconv"

	"github.com/WoW-Companion/data/internal/everloot"
)

// Npc is one row of the npcs table: client facts only (no title or gossip text, no
// spawn coordinates). Level range and faction template are null where the client lists none.
type Npc struct {
	ID                int32  `parquet:"id"`
	Name              string `parquet:"name"`
	Classification    *int8  `parquet:"classification,optional"` // client value: 0 normal, 1 elite, 2 rare elite, 3 boss, 4 rare
	CreatureType      *int8  `parquet:"creature_type,optional"`
	FamilyID          *int8  `parquet:"family_id,optional"`
	FactionTemplateID *int32 `parquet:"faction_template_id,optional"`
	MinLevel          *int16 `parquet:"min_level,optional"`
	MaxLevel          *int16 `parquet:"max_level,optional"`
	// ZoneID is a UiMap id and ZoneName that map's name; ZoneSource says where the zone came
	// from. All three are null unless an allowed source places the creature (docs/sources.md).
	ZoneID     *int32  `parquet:"zone_id,optional"`
	ZoneName   *string `parquet:"zone_name,optional"`
	ZoneSource *string `parquet:"zone_source,optional"`
	// NameSource is set on a creature the client tables lack, whose name Everloot gives;
	// null on a client row. Such a row has no classification, type, family or level.
	NameSource *string `parquet:"name_source,optional"`
}

// NameSourceEverloot is the name_source of a name taken from Everloot's drop rows.
const NameSourceEverloot = "everloot sourceName"

// ZoneSourceEverloot is the zone_source of a zone taken from Everloot's located_in relation.
const ZoneSourceEverloot = "everloot located_in"

func int8p(v int) *int8 { x := int8(v); return &x }

// optInt parses a column that may be absent or empty.
func (r Row) optInt(key string) (int, bool) {
	v := r[key]
	if v == "" {
		return 0, false
	}
	n, err := strconv.Atoi(v)
	return n, err == nil
}

// BuildNpcs joins Creature to its CreatureDifficulty rows: the level range spans every
// difficulty of the creature, the faction template is the first difficulty's (by ID) that
// lists one. Zones are left null (see ApplyNpcZones). Rows are sorted by id.
func BuildNpcs(creatures, difficulties []Row) []Npc {
	diffs := slices.Clone(difficulties)
	slices.SortStableFunc(diffs, func(a, b Row) int { return a.int("ID", 0) - b.int("ID", 0) })
	byCreature := map[int][]Row{}
	for _, d := range diffs {
		id := d.int("CreatureID", 0)
		byCreature[id] = append(byCreature[id], d)
	}
	out := make([]Npc, 0, len(creatures))
	for _, r := range creatures {
		id := r.int("ID", 0)
		n := Npc{
			ID:             int32(id),
			Name:           r["Name_lang"],
			Classification: int8p(r.int("Classification", 0)),
			CreatureType:   int8p(r.int("CreatureType", 0)),
			FamilyID:       int8p(r.int("CreatureFamily", 0)),
		}
		for _, d := range byCreature[id] {
			if f, ok := d.optInt("FactionTemplateID"); ok && f != 0 && n.FactionTemplateID == nil {
				n.FactionTemplateID = ptr(f)
			}
			lo, okLo := d.optInt("MinLevel")
			hi, okHi := d.optInt("MaxLevel")
			if !okLo || !okHi || hi == 0 {
				continue
			}
			if n.MinLevel == nil || int16(lo) < *n.MinLevel {
				v := int16(lo)
				n.MinLevel = &v
			}
			if n.MaxLevel == nil || int16(hi) > *n.MaxLevel {
				v := int16(hi)
				n.MaxLevel = &v
			}
		}
		out = append(out, n)
	}
	slices.SortStableFunc(out, func(a, b Npc) int { return int(a.ID) - int(b.ID) })
	return out
}

// AddEverlootNpcs adds a row for every creature in loot that the client rows lack and
// names has a name for, with the id and name only: the client holds nothing else about it.
// A creature with no name stays out (the name column is never empty). Rows stay sorted by id.
func AddEverlootNpcs(npcs []Npc, loot []everloot.LootSource, names map[int32]string) []Npc {
	have := map[int32]bool{}
	for _, n := range npcs {
		have[n.ID] = true
	}
	src := NameSourceEverloot
	for _, l := range loot {
		name, ok := names[l.SourceID]
		if l.SourceKind != everloot.KindCreature || have[l.SourceID] || !ok {
			continue
		}
		have[l.SourceID] = true
		npcs = append(npcs, Npc{ID: l.SourceID, Name: name, NameSource: &src})
	}
	slices.SortStableFunc(npcs, func(a, b Npc) int { return int(a.ID) - int(b.ID) })
	return npcs
}

// ApplyNpcZones fills the zone of every creature a source places. zones maps creature id to
// the UiMap id the source gives and uiMaps are the UiMap rows that name it. A creature
// without an entry keeps a null zone; an id the UiMap table does not name gets no zone_name.
func ApplyNpcZones(npcs []Npc, uiMaps []Row, zones map[int32]everloot.NpcZone) {
	names := map[int]string{}
	for _, m := range uiMaps {
		if n := m["Name_lang"]; n != "" {
			names[m.int("ID", 0)] = n
		}
	}
	for i := range npcs {
		z, ok := zones[npcs[i].ID]
		if !ok {
			continue
		}
		npcs[i].ZoneID = ptr(int(z.ZoneID))
		src := ZoneSourceEverloot
		npcs[i].ZoneSource = &src
		if name, ok := names[int(z.ZoneID)]; ok {
			npcs[i].ZoneName = &name
		}
	}
}

// ZoneCoverage counts the distinct creature ids of loot_sources, how many of them are in
// npcs, and how many of those have a zone.
func ZoneCoverage(npcs []Npc, loot []everloot.LootSource) (creatures, inNpcs, withZone int) {
	zoned := map[int32]bool{}
	for _, n := range npcs {
		zoned[n.ID] = n.ZoneID != nil
	}
	seen := map[int32]bool{}
	for _, l := range loot {
		if l.SourceKind != everloot.KindCreature || seen[l.SourceID] {
			continue
		}
		seen[l.SourceID] = true
		creatures++
		if z, ok := zoned[l.SourceID]; ok {
			inNpcs++
			if z {
				withZone++
			}
		}
	}
	return
}
