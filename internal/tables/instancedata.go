package tables

import (
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/WoW-Companion/data/internal/everloot"
)

// Sources of an instance's level range and entrance zone.
const (
	LevelClient   = "client"
	LevelCurated  = "curated"
	ZoneObserved  = "observed"
	ZoneCurated   = "curated"
	ReasonCat     = "category"
	ReasonZone    = "zone"
	curatedStatus = "advisor default"
)

func to[T any](v T) *T { return &v }

// CuratedInstance is one row of curated/instances.yaml: the typical level range and the
// entrance zone (a UiMap id and its name) of one dungeon or raid.
type CuratedInstance struct {
	MapID    int32
	Name     string
	Min, Max int
	ZoneID   int32
	Zone     string
	Source   string
}

// CuratedInstances is the parsed curated/instances.yaml.
type CuratedInstances struct {
	Status string
	Rows   []CuratedInstance
}

// ParseCuratedInstances reads the YAML subset of curated/instances.yaml: a status scalar,
// then "instances:" holding flow maps
// "- {map: 36, name: Deadmines, min: 15, max: 25, zone_id: 1436, zone: Westfall, source: X}".
func ParseCuratedInstances(file, text string) (CuratedInstances, error) {
	var c CuratedInstances
	in := false
	for i, raw := range strings.Split(text, "\n") {
		trim := strings.TrimSpace(raw)
		if trim == "" || strings.HasPrefix(trim, "#") {
			continue
		}
		fail := func(format string, a ...any) error {
			return fmt.Errorf("%s:%d: %s", file, i+1, fmt.Sprintf(format, a...))
		}
		if !strings.HasPrefix(raw, " ") {
			in = false
			k, v, ok := keyValue(trim)
			switch {
			case ok && k == "status":
				c.Status = unquote(v)
			case ok && k == "instances" && v == "":
				in = true
			default:
				return c, fail("unknown top-level line")
			}
			continue
		}
		if !in || !strings.HasPrefix(trim, "- {") || !strings.HasSuffix(trim, "}") {
			return c, fail("want a flow map - {map: N, ...} under instances:")
		}
		var r CuratedInstance
		seen := map[string]bool{}
		for _, f := range splitTop(trim[3:len(trim)-1], ',') {
			k, v, ok := keyValue(f)
			if !ok {
				return c, fail("field %q is not key: value", strings.TrimSpace(f))
			}
			seen[k] = true
			n, numErr := strconv.Atoi(v)
			switch k {
			case "map", "min", "max", "zone_id":
				if numErr != nil {
					return c, fail("%s %q is not a number", k, v)
				}
			}
			switch k {
			case "map":
				r.MapID = int32(n)
			case "min":
				r.Min = n
			case "max":
				r.Max = n
			case "zone_id":
				r.ZoneID = int32(n)
			case "name":
				r.Name = unquote(v)
			case "zone":
				r.Zone = unquote(v)
			case "source":
				r.Source = unquote(v)
			default:
				return c, fail("unknown field %q", k)
			}
		}
		for _, k := range []string{"map", "name", "min", "max", "zone_id", "zone", "source"} {
			if !seen[k] {
				return c, fail("row needs %s", k)
			}
		}
		c.Rows = append(c.Rows, r)
	}
	return c, nil
}

// LoadCuratedInstances reads and parses one curated instances file. A missing file is an
// error: the build needs it.
func LoadCuratedInstances(path string) (CuratedInstances, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return CuratedInstances{}, err
	}
	return ParseCuratedInstances(path, string(raw))
}

// ApplyInstanceData fills level ranges and entrance zones the client lacks. Levels: the
// client's content tuning range stays; otherwise the curated range. Entrance zone: an
// Everloot-observed zone (corroborated, see everloot.BuildEntranceZones) wins over the
// curated one, and its name is the UiMap table's. A curated row must name an instance in
// instances, carry that instance's name, have min <= max, name a UiMap with the given name,
// and its file must mark itself an advisor default; any problem is an error.
func ApplyInstanceData(instances []Instance, cur CuratedInstances, uiMaps []Row, observed map[int32]everloot.EntranceZone) error {
	zoneName := map[int32]string{}
	for _, m := range uiMaps {
		if n := m["Name_lang"]; n != "" {
			zoneName[int32(m.int("ID", 0))] = n
		}
	}
	if !strings.Contains(cur.Status, curatedStatus) {
		return fmt.Errorf("curated instances: status must say %q", curatedStatus)
	}
	byMap := map[int32]CuratedInstance{}
	var errs []string
	for _, r := range cur.Rows {
		i := slices.IndexFunc(instances, func(x Instance) bool { return x.MapID == r.MapID })
		switch {
		case i < 0:
			errs = append(errs, fmt.Sprintf("map %d (%s) is not an instance", r.MapID, r.Name))
			continue
		case instances[i].Name != r.Name:
			errs = append(errs, fmt.Sprintf("map %d is %q, not %q", r.MapID, instances[i].Name, r.Name))
		case r.Min < 1 || r.Min > r.Max:
			errs = append(errs, fmt.Sprintf("map %d: level range %d-%d", r.MapID, r.Min, r.Max))
		case r.Source == "":
			errs = append(errs, fmt.Sprintf("map %d: no source", r.MapID))
		}
		if n, ok := zoneName[r.ZoneID]; !ok || n != r.Zone {
			errs = append(errs, fmt.Sprintf("map %d: zone %d is %q in UiMap, not %q", r.MapID, r.ZoneID, n, r.Zone))
		}
		if _, dup := byMap[r.MapID]; dup {
			errs = append(errs, fmt.Sprintf("map %d listed twice", r.MapID))
		}
		byMap[r.MapID] = r
	}
	if len(errs) > 0 {
		return fmt.Errorf("curated instances: %s", strings.Join(errs, "; "))
	}
	for i := range instances {
		in := &instances[i]
		c, hasCur := byMap[in.MapID]
		if in.LevelSource == nil && hasCur {
			in.MinLevel, in.MaxLevel, in.LevelSource = to(int16(c.Min)), to(int16(c.Max)), to(LevelCurated)
		}
		if z, ok := observed[in.MapID]; ok {
			if n, named := zoneName[z.UIMapID]; named {
				in.EntranceZoneID, in.EntranceZoneName, in.EntranceZoneSource = to(z.UIMapID), to(n), to(ZoneObserved)
				continue
			}
		}
		if hasCur {
			in.EntranceZoneID, in.EntranceZoneName, in.EntranceZoneSource = to(c.ZoneID), to(c.Zone), to(ZoneCurated)
		}
	}
	return nil
}

// ApplyEncounterObs sets Reporters on every boss an encounter corroborates (same map and
// encounter id, at least two installs).
func ApplyEncounterObs(bosses []InstanceBoss, obs map[everloot.EncounterKey]int32) int {
	n := 0
	for i := range bosses {
		if r, ok := obs[everloot.EncounterKey{MapID: bosses[i].MapID, EncounterID: bosses[i].EncounterID}]; ok {
			bosses[i].Reporters = to(r)
			n++
		}
	}
	return n
}

// TagInstanceLoot sets MapID on every creature loot row whose creature is the linked boss
// of exactly one instance, and returns how many rows it tagged. A creature that is a boss
// of two maps is left untagged.
func TagInstanceLoot(rows []everloot.LootSource, bosses []InstanceBoss) int {
	maps := map[int32]int32{}
	ambiguous := map[int32]bool{}
	for _, b := range bosses {
		if b.CreatureID == nil {
			continue
		}
		if m, ok := maps[*b.CreatureID]; ok && m != b.MapID {
			ambiguous[*b.CreatureID] = true
		}
		maps[*b.CreatureID] = b.MapID
	}
	n := 0
	for i := range rows {
		if rows[i].SourceKind != everloot.KindCreature || ambiguous[rows[i].SourceID] {
			continue
		}
		if m, ok := maps[rows[i].SourceID]; ok {
			rows[i].MapID = to(m)
			n++
		}
	}
	return n
}

// InstanceQuest is one row of instance_quests: a quest to pick up before entering an
// instance. Reason is category (an observed quest's log heading names the instance) or zone
// (the client's quest zone names it).
type InstanceQuest struct {
	MapID   int32  `parquet:"map_id"`
	QuestID int32  `parquet:"quest_id"`
	Reason  string `parquet:"reason,dict"`
}

func instanceKey(name string) string {
	k := strings.ToLower(strings.TrimSpace(name))
	return strings.TrimSpace(strings.TrimPrefix(k, "the "))
}

// BuildInstanceQuests ties quests to the instance their heading or zone names. A name shared
// by two instances (the client lists Naxxramas twice) counts only when exactly one of them
// has a level range. Rows are sorted by map, quest and reason; a quest named by both sources
// has one row per reason.
func BuildInstanceQuests(instances []Instance, observed []everloot.QuestObserved, quests []Quest) []InstanceQuest {
	by := map[string][]Instance{}
	for _, in := range instances {
		by[instanceKey(in.Name)] = append(by[instanceKey(in.Name)], in)
	}
	resolve := func(name string) (int32, bool) {
		c := by[instanceKey(name)]
		if len(c) > 1 {
			c = slices.DeleteFunc(slices.Clone(c), func(i Instance) bool { return i.LevelSource == nil })
		}
		if len(c) != 1 {
			return 0, false
		}
		return c[0].MapID, true
	}
	out := []InstanceQuest{}
	for _, q := range observed {
		if q.Category == nil {
			continue
		}
		if m, ok := resolve(*q.Category); ok {
			out = append(out, InstanceQuest{MapID: m, QuestID: q.QuestID, Reason: ReasonCat})
		}
	}
	for _, q := range quests {
		if q.ZoneName == nil {
			continue
		}
		if m, ok := resolve(*q.ZoneName); ok {
			out = append(out, InstanceQuest{MapID: m, QuestID: q.ID, Reason: ReasonZone})
		}
	}
	slices.SortFunc(out, func(a, b InstanceQuest) int {
		if a.MapID != b.MapID {
			return int(a.MapID - b.MapID)
		}
		if a.QuestID != b.QuestID {
			return int(a.QuestID - b.QuestID)
		}
		return strings.Compare(a.Reason, b.Reason)
	})
	return out
}

// InstanceCoverage is a one-line summary of what the instance tables hold, for the build log.
func InstanceCoverage(instances []Instance, bosses []InstanceBoss, loot []everloot.LootSource, quests []InstanceQuest, corroborated, tagged int) string {
	var lv, zn, lk int
	for _, i := range instances {
		if i.LevelSource != nil {
			lv++
		}
		if i.EntranceZoneID != nil {
			zn++
		}
	}
	for _, b := range bosses {
		if b.CreatureID != nil {
			lk++
		}
	}
	return fmt.Sprintf("%d with a level range, %d with an entrance zone, %d bosses linked of %d (%d corroborated by Everloot), %d boss loot rows, %d instance quests",
		lv, zn, lk, len(bosses), corroborated, tagged, len(quests))
}
