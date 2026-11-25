package everloot

import (
	"encoding/json"
	"fmt"
	"github.com/WoW-Companion/data/internal/names"
	"os"
	"path/filepath"
	"slices"
)

// FetchLoot makes FetchCorpus also read the loot windows (24,996 records, 50 pages): the
// source of gathering and fishing observations. It is off by default because 50 pages
// exceed Everloot's 30 requests an hour without a key.
var FetchLoot = false

// Rollup route and cache file of the disenchant answer.
const (
	DisenchantPath = "/api/rollups/disenchant"
	DisenchantFile = "disenchant.json"
	// LootGather and LootFishing are the lootType values of a gathering and a fishing window.
	LootGather  = "gather"
	LootFishing = "fishing"
	// Gather kinds Everloot labels a node with.
	GatherOre  = "ore"
	GatherHerb = "herb"
)

// TrainerRecord is one public record of kind trainer_seen: one recipe offered by one
// trainer to one install. The recipe is named in the install's language, not by spell id.
type TrainerRecord struct {
	InstallID    string `json:"installId"`
	Profession   string `json:"profession"`
	Recipe       string `json:"recipe"`
	RequiredRank *int32 `json:"requiredRank,omitempty"`
	CostCopper   *int64 `json:"costCopper,omitempty"`
	TrainerNpcID int32  `json:"trainerNpcId"`
}

// LootItem is one item of a loot window.
type LootItem struct {
	ItemID int32 `json:"itemId"`
	Count  int32 `json:"count"`
}

// LootRecord is one public record of kind loot. Only the fields read are decoded.
type LootRecord struct {
	InstallID        string     `json:"installId"`
	LootType         string     `json:"lootType"`
	SourceObjectID   int32      `json:"sourceObjectId,omitempty"`
	SourceObjectName string     `json:"sourceObjectName,omitempty"`
	GatherKind       string     `json:"gatherKind,omitempty"`
	MapID            int32      `json:"mapId,omitempty"`
	Items            []LootItem `json:"items"`
}

// TrainerOffer is one corroborated recipe a trainer teaches. Cost and rank are null unless
// at least MinReporters installs agree on the value.
type TrainerOffer struct {
	NpcID         int32
	Profession    string
	Recipe        string
	CostCopper    *int64
	RequiredSkill *int16
	Reporters     int32
}

// BuildTrainerOffers keeps an offer (trainer, profession, recipe name) that at least
// MinReporters distinct installs saw. Sorted by trainer, profession and recipe.
func BuildTrainerOffers(c Corpus) []TrainerOffer {
	type key struct {
		npc       int32
		prof, rec string
	}
	type agg struct {
		installs map[string]struct{}
		cost     vote[int64]
		rank     vote[int16]
	}
	by := map[key]*agg{}
	for _, r := range c.Trainers {
		if r.TrainerNpcID <= 0 || r.InstallID == "" || r.Profession == "" || r.Recipe == "" {
			continue
		}
		k := key{r.TrainerNpcID, r.Profession, r.Recipe}
		a := by[k]
		if a == nil {
			a = &agg{installs: map[string]struct{}{}, cost: vote[int64]{}, rank: vote[int16]{}}
			by[k] = a
		}
		a.installs[r.InstallID] = struct{}{}
		if r.CostCopper != nil && *r.CostCopper >= 0 {
			a.cost.add(*r.CostCopper, r.InstallID)
		}
		if r.RequiredRank != nil && *r.RequiredRank > 0 {
			a.rank.add(int16(*r.RequiredRank), r.InstallID)
		}
	}
	out := []TrainerOffer{}
	for k, a := range by {
		if len(a.installs) < MinReporters {
			continue
		}
		o := TrainerOffer{NpcID: k.npc, Profession: k.prof, Recipe: k.rec, Reporters: int32(len(a.installs))}
		if v, ok := pick(a.cost, false, func(x, y int64) bool { return x < y }); ok {
			o.CostCopper = ptr(v)
		}
		if v, ok := pick(a.rank, false, lessI16); ok {
			o.RequiredSkill = ptr(v)
		}
		out = append(out, o)
	}
	slices.SortFunc(out, func(a, b TrainerOffer) int {
		if a.NpcID != b.NpcID {
			return int(a.NpcID - b.NpcID)
		}
		if a.Profession != b.Profession {
			return cmpStr(a.Profession, b.Profession)
		}
		return cmpStr(a.Recipe, b.Recipe)
	})
	return out
}

func cmpStr(a, b string) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// NodeObs is one gathering node in one zone that at least MinReporters installs looted.
type NodeObs struct {
	ObjectID   int32
	ZoneID     int32 // a UiMap id
	Name       string
	GatherKind string
	Windows    int32
	Reporters  int32
}

// NodeYield is one item a node gave, in one or more windows.
type NodeYield struct {
	ObjectID  int32
	ItemID    int32
	Windows   int32
	Reporters int32
	MinCount  int32
	MaxCount  int32
}

// BuildGatherObs reads gather loot windows of an object. A node-zone pair is kept at
// MinReporters distinct installs; its name and gather kind are the values most installs
// reported. A yield is kept when at least MinReporters installs saw the item from the node.
func BuildGatherObs(c Corpus) ([]NodeObs, []NodeYield) {
	type zk struct{ obj, zone int32 }
	type yk struct{ obj, item int32 }
	type nodeAgg struct {
		installs map[string]struct{}
		windows  int32
		name     vote[string]
		kind     vote[string]
	}
	type yAgg struct {
		installs map[string]struct{}
		windows  int32
		lo, hi   int32
	}
	nodes := map[zk]*nodeAgg{}
	yields := map[yk]*yAgg{}
	for _, r := range c.Loots {
		if r.LootType != LootGather || r.SourceObjectID <= 0 || r.MapID <= 0 || r.InstallID == "" {
			continue
		}
		k := zk{r.SourceObjectID, r.MapID}
		n := nodes[k]
		if n == nil {
			n = &nodeAgg{installs: map[string]struct{}{}, name: vote[string]{}, kind: vote[string]{}}
			nodes[k] = n
		}
		n.installs[r.InstallID] = struct{}{}
		n.windows++
		if r.SourceObjectName != "" {
			n.name.add(r.SourceObjectName, r.InstallID)
		}
		if r.GatherKind != "" {
			n.kind.add(r.GatherKind, r.InstallID)
		}
		for _, it := range r.Items {
			if it.ItemID <= 0 || it.Count <= 0 {
				continue
			}
			y := yields[yk{r.SourceObjectID, it.ItemID}]
			if y == nil {
				y = &yAgg{installs: map[string]struct{}{}, lo: it.Count, hi: it.Count}
				yields[yk{r.SourceObjectID, it.ItemID}] = y
			}
			y.installs[r.InstallID] = struct{}{}
			y.windows++
			y.lo, y.hi = min(y.lo, it.Count), max(y.hi, it.Count)
		}
	}
	var no []NodeObs
	for k, n := range nodes {
		if len(n.installs) < MinReporters {
			continue
		}
		o := NodeObs{ObjectID: k.obj, ZoneID: k.zone, Windows: n.windows, Reporters: int32(len(n.installs))}
		o.Name, _ = best(n.name, lessS)
		if o.Name != "" && !names.IsEnglish(o.Name) {
			DroppedNames.Add(1)
			o.Name = ""
		}
		o.GatherKind, _ = best(n.kind, lessS)
		no = append(no, o)
	}
	slices.SortFunc(no, func(a, b NodeObs) int {
		if a.ObjectID != b.ObjectID {
			return int(a.ObjectID - b.ObjectID)
		}
		return int(a.ZoneID - b.ZoneID)
	})
	var yo []NodeYield
	for k, y := range yields {
		if len(y.installs) >= MinReporters {
			yo = append(yo, NodeYield{ObjectID: k.obj, ItemID: k.item, Windows: y.windows, Reporters: int32(len(y.installs)), MinCount: y.lo, MaxCount: y.hi})
		}
	}
	slices.SortFunc(yo, func(a, b NodeYield) int {
		if a.ObjectID != b.ObjectID {
			return int(a.ObjectID - b.ObjectID)
		}
		return int(a.ItemID - b.ItemID)
	})
	return no, yo
}

// FishCatch is how often an item came out of a fishing window in a zone.
type FishCatch struct {
	ZoneID    int32 // a UiMap id
	ItemID    int32
	Casts     int32 // fishing windows seen in the zone
	Catches   int32 // windows holding the item
	Reporters int32
	Rate      float64 // Catches / Casts
}

// BuildFishing reads fishing loot windows (lootType fishing, or gatherKind fishing). An item
// in a zone is kept when at least MinReporters distinct installs caught it there.
func BuildFishing(c Corpus) []FishCatch {
	type ik struct{ zone, item int32 }
	casts := map[int32]int32{}
	type agg struct {
		installs map[string]struct{}
		n        int32
	}
	by := map[ik]*agg{}
	for _, r := range c.Loots {
		if (r.LootType != LootFishing && r.GatherKind != LootFishing) || r.MapID <= 0 || r.InstallID == "" {
			continue
		}
		casts[r.MapID]++
		seen := map[int32]bool{}
		for _, it := range r.Items {
			if it.ItemID <= 0 || seen[it.ItemID] {
				continue
			}
			seen[it.ItemID] = true
			a := by[ik{r.MapID, it.ItemID}]
			if a == nil {
				a = &agg{installs: map[string]struct{}{}}
				by[ik{r.MapID, it.ItemID}] = a
			}
			a.installs[r.InstallID] = struct{}{}
			a.n++
		}
	}
	var out []FishCatch
	for k, a := range by {
		if len(a.installs) < MinReporters {
			continue
		}
		out = append(out, FishCatch{ZoneID: k.zone, ItemID: k.item, Casts: casts[k.zone], Catches: a.n,
			Reporters: int32(len(a.installs)), Rate: float64(a.n) / float64(casts[k.zone])})
	}
	slices.SortFunc(out, func(a, b FishCatch) int {
		if a.ZoneID != b.ZoneID {
			return int(a.ZoneID - b.ZoneID)
		}
		return int(a.ItemID - b.ItemID)
	})
	return out
}

// DisenchantMaterial is one material row of a disenchant band.
type DisenchantMaterial struct {
	ItemID        int32   `json:"itemId"`
	WithMaterial  int32   `json:"withMaterial"`
	Chance        float64 `json:"chance"`
	PerDisenchant float64 `json:"perDisenchant"`
	Min           int32   `json:"min"`
	Max           int32   `json:"max"`
}

// DisenchantBand is one band of /api/rollups/disenchant: an item class and quality over an
// item level range.
type DisenchantBand struct {
	BandID    int32                `json:"bandId"`
	ItemClass string               `json:"itemClass"`
	Quality   string               `json:"quality"`
	MinLevel  int32                `json:"minLevel"`
	MaxLevel  int32                `json:"maxLevel"`
	State     string               `json:"state"`
	Windows   int32                `json:"windows"`
	Installs  int32                `json:"installs"`
	Materials []DisenchantMaterial `json:"materials"`
}

// Disenchant is the cached rollup.
type Disenchant struct {
	Retrieved   string           `json:"retrieved"`
	DataAsOf    string           `json:"dataAsOf"`
	Build       string           `json:"build"`
	Attribution Attribution      `json:"attribution"`
	Bands       []DisenchantBand `json:"bands"`
}

// FetchDisenchant downloads the rollup (one request) into cacheDir/disenchant.json; an
// existing cache file is kept as is.
func FetchDisenchant(baseURL, cacheDir, retrieved string) (Disenchant, error) {
	path := filepath.Join(cacheDir, DisenchantFile)
	if _, err := os.Stat(path); err == nil {
		return LoadDisenchant(path)
	}
	d := Disenchant{Retrieved: retrieved}
	body, err := newGetter().Get(baseURL + DisenchantPath)
	if err != nil {
		return d, err
	}
	if err := RequireFields("everloot disenchant", "bands", []string{"bandId", "itemClass", "quality", "minLevel", "maxLevel", "state"}, body); err != nil {
		return d, err
	}
	if err := json.Unmarshal(body, &d); err != nil {
		return d, fmt.Errorf("everloot disenchant: %w", err)
	}
	d.Retrieved = retrieved
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return d, err
	}
	b, _ := json.MarshalIndent(d, "", " ")
	return d, os.WriteFile(path, append(b, '\n'), 0o644)
}

// LoadDisenchant reads a cached rollup; a missing file is an empty rollup.
func LoadDisenchant(path string) (Disenchant, error) {
	var d Disenchant
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return d, nil
	}
	if err != nil {
		return d, err
	}
	return d, json.Unmarshal(b, &d)
}

// DisenchantYield is one row of disenchant_yields.
type DisenchantYield struct {
	BandID        int32   `parquet:"band_id"`
	ItemClass     string  `parquet:"item_class,dict"`
	Quality       string  `parquet:"quality,dict"`
	MinItemLevel  int32   `parquet:"min_item_level"`
	MaxItemLevel  int32   `parquet:"max_item_level"`
	State         string  `parquet:"state,dict"`
	Windows       int32   `parquet:"windows"`
	Installs      int32   `parquet:"installs"`
	MaterialID    int32   `parquet:"material_item_id"`
	Chance        float64 `parquet:"chance"`
	PerDisenchant float64 `parquet:"per_disenchant"`
	MinCount      int32   `parquet:"min_count"`
	MaxCount      int32   `parquet:"max_count"`
}

// BuildDisenchantYields makes one row per material of each band that lists materials,
// kept exactly as Everloot gives them. Sorted by band and material.
func BuildDisenchantYields(d Disenchant) []DisenchantYield {
	out := []DisenchantYield{}
	for _, b := range d.Bands {
		for _, m := range b.Materials {
			if m.ItemID <= 0 {
				continue
			}
			out = append(out, DisenchantYield{BandID: b.BandID, ItemClass: b.ItemClass, Quality: b.Quality, MinItemLevel: b.MinLevel,
				MaxItemLevel: b.MaxLevel, State: b.State, Windows: b.Windows, Installs: b.Installs, MaterialID: m.ItemID,
				Chance: m.Chance, PerDisenchant: m.PerDisenchant, MinCount: m.Min, MaxCount: m.Max})
		}
	}
	slices.SortFunc(out, func(a, b DisenchantYield) int {
		if a.BandID != b.BandID {
			return int(a.BandID - b.BandID)
		}
		return int(a.MaterialID - b.MaterialID)
	})
	return out
}
