package tables

import (
	"slices"
	"strings"
)

// Instance is one row of the instances table: a client map whose instance type is dungeon
// (1) or raid (2). Entrance fields are the map's own corpse (entrance) map and position,
// null for maps that name none. Levels come from the content tuning row of the map's
// difficulties and are null when that row holds none; nothing is inferred.
type Instance struct {
	MapID         int32    `parquet:"map_id"`
	Name          string   `parquet:"name"`
	InstanceType  string   `parquet:"instance_type"`
	MaxPlayers    *int16   `parquet:"max_players,optional"`
	EntranceMapID *int32   `parquet:"entrance_map_id,optional"`
	EntranceX     *float32 `parquet:"entrance_x,optional"`
	EntranceY     *float32 `parquet:"entrance_y,optional"`
	MinLevel      *int16   `parquet:"min_level,optional"`
	MaxLevel      *int16   `parquet:"max_level,optional"`
	// LevelSource says where MinLevel and MaxLevel came from: client or curated (observed is
	// reserved: no Everloot record carries an instance level).
	LevelSource *string `parquet:"level_source,optional,dict"`
	// EntranceZoneID and EntranceZoneName are the open-world UiMap the entrance stands on;
	// EntranceZoneSource is observed (Everloot) or curated.
	EntranceZoneID     *int32  `parquet:"entrance_zone_id,optional"`
	EntranceZoneName   *string `parquet:"entrance_zone_name,optional,dict"`
	EntranceZoneSource *string `parquet:"entrance_zone_source,optional,dict"`
}

// InstanceBoss is one row of the instance_bosses table: a client dungeon encounter.
// CreatureID is set by LinkBosses, only when exactly one npc carries the encounter's exact name.
type InstanceBoss struct {
	MapID        int32  `parquet:"map_id"`
	EncounterID  int32  `parquet:"encounter_id"`
	Name         string `parquet:"name"`
	OrderIndex   int32  `parquet:"order_index"`
	DifficultyID int32  `parquet:"difficulty_id"`
	CreatureID   *int32 `parquet:"creature_id,optional"`
	// Reporters is how many distinct Everloot installs killed the encounter; null when
	// fewer than two did.
	Reporters *int32 `parquet:"reporters,optional"`
}

// InstanceInput is the client tables the instance builders read.
type InstanceInput struct {
	Maps, Encounters, MapDifficulties, ContentTuning []Row
}

const (
	instanceDungeon = "dungeon"
	instanceRaid    = "raid"
)

// instanceTypeOf maps Map.InstanceType (1 dungeon, 2 raid) to a label; other types
// (open world, battleground, arena) are not instances here.
func instanceTypeOf(r Row) string {
	switch r.int("InstanceType", 0) {
	case 1:
		return instanceDungeon
	case 2:
		return instanceRaid
	}
	return ""
}

// isPlaceholderMap marks the client's test and unused maps.
func isPlaceholderMap(name string) bool {
	return name == "" || strings.HasPrefix(name, "<") || strings.Contains(name, "Test")
}

func optPositive(v int) *int16 {
	if v <= 0 {
		return nil
	}
	x := int16(v)
	return &x
}

// BuildInstances makes one row per dungeon or raid map, sorted by map id. The level range is
// the content tuning LfgMinLevel and LfgMaxLevel of the map's lowest-id difficulty that has
// a non-zero pair; zero means none and is stored as null.
func BuildInstances(in InstanceInput) []Instance {
	tuning := map[int]Row{}
	for _, r := range in.ContentTuning {
		tuning[r.int("ID", 0)] = r
	}
	diffs := slices.Clone(in.MapDifficulties)
	slices.SortStableFunc(diffs, func(a, b Row) int { return a.int("ID", 0) - b.int("ID", 0) })
	levels := map[int][2]int{}
	for _, d := range diffs {
		t, ok := tuning[d.int("ContentTuningID", 0)]
		lo, hi := t.int("LfgMinLevel", 0), t.int("LfgMaxLevel", 0)
		m := d.int("MapID", 0)
		if _, done := levels[m]; ok && !done && lo > 0 && hi > 0 {
			levels[m] = [2]int{lo, hi}
		}
	}
	out := []Instance{}
	for _, r := range in.Maps {
		kind, name := instanceTypeOf(r), r["MapName_lang"]
		if kind == "" || isPlaceholderMap(name) {
			continue
		}
		id := r.int("ID", 0)
		inst := Instance{MapID: int32(id), Name: name, InstanceType: kind, MaxPlayers: optPositive(r.int("MaxPlayers", 0))}
		if e := r.int("CorpseMapID", -1); e >= 0 {
			inst.EntranceMapID = ptr(e)
			if x, y := parseFloat(r["Corpse_0"]), parseFloat(r["Corpse_1"]); x != 0 || y != 0 {
				inst.EntranceX, inst.EntranceY = &x, &y
			}
		}
		if l, ok := levels[id]; ok {
			inst.MinLevel, inst.MaxLevel = optPositive(l[0]), optPositive(l[1])
			if inst.MinLevel != nil && inst.MaxLevel != nil {
				inst.LevelSource = to(LevelClient)
			}
		}
		out = append(out, inst)
	}
	slices.SortFunc(out, func(a, b Instance) int { return int(a.MapID - b.MapID) })
	return out
}

// BuildInstanceBosses makes one row per dungeon encounter of an instance in instances,
// sorted by map, order index and encounter id. CreatureID is left null (see LinkBosses).
func BuildInstanceBosses(instances []Instance, in InstanceInput) []InstanceBoss {
	maps := map[int32]bool{}
	for _, i := range instances {
		maps[i.MapID] = true
	}
	out := []InstanceBoss{}
	for _, r := range in.Encounters {
		m := int32(r.int("MapID", 0))
		name := r["Name_lang"]
		if !maps[m] || name == "" {
			continue
		}
		b := InstanceBoss{
			MapID: m, EncounterID: int32(r.int("ID", 0)), Name: name,
			OrderIndex: int32(r.int("OrderIndex", 0)), DifficultyID: int32(r.int("DifficultyID", 0)),
		}
		out = append(out, b)
	}
	slices.SortFunc(out, func(a, b InstanceBoss) int {
		if a.MapID != b.MapID {
			return int(a.MapID - b.MapID)
		}
		if a.OrderIndex != b.OrderIndex {
			return int(a.OrderIndex - b.OrderIndex)
		}
		return int(a.EncounterID - b.EncounterID)
	})
	return out
}

// LinkBosses sets CreatureID on every boss whose exact name is carried by exactly one npc,
// and returns how many it linked. No client table or Everloot relation ties an encounter to
// a creature, so the name is the only key. When several npcs share the name, the ones that
// are a loot source (hasLoot, by creature id) decide: exactly one of them links, none or
// several link nothing.
func LinkBosses(bosses []InstanceBoss, npcs []Npc, hasLoot map[int32]bool) int {
	byName := map[string][]int32{}
	for _, n := range npcs {
		byName[n.Name] = append(byName[n.Name], n.ID)
	}
	linked := 0
	for i := range bosses {
		ids := byName[bosses[i].Name]
		if len(ids) > 1 {
			var withLoot []int32
			for _, id := range ids {
				if hasLoot[id] {
					withLoot = append(withLoot, id)
				}
			}
			ids = withLoot
		}
		if len(ids) == 1 {
			bosses[i].CreatureID = &ids[0]
			linked++
		}
	}
	return linked
}
