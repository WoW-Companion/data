package tables

import (
	"slices"
	"strconv"
)

// Npc is one row of the npcs table: client facts only (no title or gossip text, no
// spawn coordinates). Level range and faction are null where the client lists none.
type Npc struct {
	ID             int32  `parquet:"id"`
	Name           string `parquet:"name"`
	Classification int8   `parquet:"classification"` // client value: 0 normal, 1 elite, 2 rare elite, 3 boss, 4 rare
	CreatureType   int8   `parquet:"creature_type"`
	FamilyID       int8   `parquet:"family_id"`
	FactionID      *int32 `parquet:"faction_id,optional"`
	MinLevel       *int16 `parquet:"min_level,optional"`
	MaxLevel       *int16 `parquet:"max_level,optional"`
}

// optInt parses a column that may be absent or empty.
func (r Row) optInt(key string) (int, bool) {
	v := r[key]
	if v == "" {
		return 0, false
	}
	n, err := strconv.Atoi(v)
	return n, err == nil
}

// BuildNpcs joins Creature to its CreatureDifficulty rows and their ContentTuning level
// ranges: the range spans every difficulty of the creature, the faction is the first
// difficulty's (by ID) that lists one. Rows are sorted by id.
func BuildNpcs(creatures, difficulties, tunings []Row) []Npc {
	tuning := make(map[int]Row, len(tunings))
	for _, t := range tunings {
		tuning[t.int("ID", 0)] = t
	}
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
			if f, ok := d.optInt("FactionID"); ok && f != 0 && n.FactionID == nil {
				n.FactionID = ptr(f)
			}
			t, ok := tuning[d.int("ContentTuningID", 0)]
			if !ok {
				continue
			}
			lo, okLo := t.optInt("MinLevel")
			hi, okHi := t.optInt("MaxLevel")
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
