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
	Classification    int8   `parquet:"classification"` // client value: 0 normal, 1 elite, 2 rare elite, 3 boss, 4 rare
	CreatureType      int8   `parquet:"creature_type"`
	FamilyID          int8   `parquet:"family_id"`
	FactionTemplateID *int32 `parquet:"faction_template_id,optional"`
	MinLevel          *int16 `parquet:"min_level,optional"`
	MaxLevel          *int16 `parquet:"max_level,optional"`
	// ZoneID is a UiMap id and ZoneName that map's name; ZoneSource says where the zone came
	// from. All three are null unless an allowed source places the creature (docs/sources.md).
	ZoneID     *int32  `parquet:"zone_id,optional"`
	ZoneName   *string `parquet:"zone_name,optional"`
	ZoneSource *string `parquet:"zone_source,optional"`
}

// ZoneSourceEverloot is the zone_source of a zone taken from Everloot's located_in relation.
const ZoneSourceEverloot = "everloot located_in"

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
			Classification: int8(r.int("Classification", 0)),
			CreatureType:   int8(r.int("CreatureType", 0)),
			FamilyID:       int8(r.int("CreatureFamily", 0)),
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

// ZoneCoverage counts the creature rows of loot_sources whose creature is in npcs, and how
// many of those have a zone.
func ZoneCoverage(npcs []Npc, loot []everloot.LootSource) (creatureRows, inNpcs, withZone int) {
	zoned := map[int32]bool{}
	for _, n := range npcs {
		zoned[n.ID] = n.ZoneID != nil
	}
	for _, l := range loot {
		if l.SourceKind != everloot.KindCreature {
			continue
		}
		creatureRows++
		if z, ok := zoned[l.SourceID]; ok {
			inNpcs++
			if z {
				withZone++
			}
		}
	}
	return
}
