package everloot

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"time"
)

// Corpus kinds and routes read for observed quests and vendor offers.
const (
	CorpusPath    = "/api/corpus/public"
	SettledPath   = "/api/coverage/settled"
	CorpusFile    = "corpus.json"
	KindQuestRec  = "quest"
	KindVendorRec = "vendor"
	// KindEncounterRec and KindEntranceRec are the instance records read for boss
	// corroboration and entrance zones.
	KindEncounterRec = "encounter"
	KindEntranceRec  = "instance_entrance"
	// KindTrainerRec is a trainer window an install saw; KindLootRec is a loot window
	// (fetched only when FetchLoot is set: it is 50 pages).
	KindTrainerRec = "trainer_seen"
	KindLootRec    = "loot"
	// KindSoldBy is the loot_sources source_kind of a vendor offer; source_id is the vendor npc.
	KindSoldBy = "sold by"
	// MinReporters is how many distinct installs must report the same value before an
	// unsettled record is kept (Everloot calls a fact settled at five agreeing installs).
	MinReporters = 2
	// TierSettled and TierCorroborated are the tier of a quest reward or vendor loot row.
	TierSettled      = "settled"
	TierCorroborated = "corroborated"
)

// QuestRecord is one public record of kind quest: what one install saw. Only facts the
// dataset keeps are decoded; installId is Everloot's per-install pseudonym.
type QuestRecord struct {
	InstallID     string  `json:"installId"`
	QuestID       int32   `json:"questId"`
	Title         string  `json:"title"`
	Level         *int16  `json:"level,omitempty"`
	GiverNpcID    int32   `json:"giverNpcId,omitempty"`
	TurnInNpcID   int32   `json:"turnInNpcId,omitempty"`
	RewardItemIDs []int32 `json:"rewardItemIds,omitempty"`
	Category      string  `json:"category,omitempty"`
	Faction       string  `json:"faction,omitempty"`
	Group         *int16  `json:"suggestedGroup,omitempty"`
	Tag           string  `json:"tag,omitempty"`
}

// VendorItem is one offer of a vendor record.
type VendorItem struct {
	ItemID       int32  `json:"itemId"`
	PriceCopper  int64  `json:"priceCopper"`
	LimitedStock *int32 `json:"limitedStock,omitempty"`
}

// VendorRecord is one public record of kind vendor.
type VendorRecord struct {
	InstallID   string       `json:"installId"`
	VendorNpcID int32        `json:"vendorNpcId"`
	Items       []VendorItem `json:"items"`
}

// Corpus is the cached union of the quest and vendor records and the settled lists.
type Corpus struct {
	Retrieved   string            `json:"retrieved"`
	DataAsOf    string            `json:"dataAsOf"`
	Build       string            `json:"build"`
	Attribution Attribution       `json:"attribution"`
	Quests      []QuestRecord     `json:"quests"`
	Vendors     []VendorRecord    `json:"vendors"`
	Encounters  []EncounterRecord `json:"encounters,omitempty"`
	Entrances   []EntranceRecord  `json:"entrances,omitempty"`
	Trainers    []TrainerRecord   `json:"trainers,omitempty"`
	Loots       []LootRecord      `json:"loots,omitempty"`
	// SettledQuests and SettledVendors are decoded ids (sorted ascending).
	SettledQuests  []int32 `json:"settledQuests,omitempty"`
	SettledVendors []int32 `json:"settledVendors,omitempty"`
}

type corpusPage struct {
	DataAsOf    string            `json:"dataAsOf"`
	Attribution Attribution       `json:"attribution"`
	Records     []json.RawMessage `json:"records"`
	Next        *string           `json:"next"`
}

type settledList struct {
	Build    string  `json:"build"`
	DataAsOf string  `json:"dataAsOf"`
	Quest    []int64 `json:"quest"`
	Vendor   []int64 `json:"vendor"`
}

// DecodeDelta turns a sorted, delta-encoded id list (first entry the id, each later one the
// gap to the one before) into ids. A negative entry makes the list invalid.
func DecodeDelta(d []int64) ([]int32, error) {
	out := make([]int32, 0, len(d))
	var cur int64
	for i, g := range d {
		if g < 0 || (i > 0 && g == 0) {
			return nil, fmt.Errorf("settled list entry %d is %d: not a delta-encoded ascending list", i, g)
		}
		cur += g
		out = append(out, int32(cur))
	}
	return out, nil
}

// LoadCorpus reads a cached corpus; a missing file is an empty corpus, not an error.
func LoadCorpus(path string) (Corpus, error) {
	var c Corpus
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	return c, json.Unmarshal(b, &c)
}

// FetchCorpus downloads every page of quest and vendor records plus the settled lists of
// build, and writes cacheDir/corpus.json. An existing cache file is kept as is. Any error
// stops the fetch.
func FetchCorpus(baseURL, cacheDir, retrieved, build string) (Corpus, error) {
	path := filepath.Join(cacheDir, CorpusFile)
	if _, err := os.Stat(path); err == nil {
		return LoadCorpus(path)
	}
	c := Corpus{Retrieved: retrieved, Build: build}
	client := newGetter()
	kinds := []string{KindQuestRec, KindVendorRec, KindEncounterRec, KindEntranceRec, KindTrainerRec}
	if FetchLoot || apiKey() != "" { // with a key the 50 loot pages fit the hourly limit
		kinds = append(kinds, KindLootRec)
	}
	for _, kind := range kinds {
		u := fmt.Sprintf("%s%s?kinds=%s&offset=0&limit=%d", baseURL, CorpusPath, kind, pageLen())
		for u != "" {
			time.Sleep(Delay)
			body, err := client.Get(u)
			if err != nil {
				return c, err
			}
			var p corpusPage
			if err := json.Unmarshal(body, &p); err != nil {
				return c, fmt.Errorf("everloot corpus %s: %w", kind, err)
			}
			c.DataAsOf, c.Attribution = p.DataAsOf, p.Attribution
			for _, raw := range p.Records {
				if err := c.add(kind, raw); err != nil {
					return c, err
				}
			}
			u = ""
			if p.Next != nil {
				u = *p.Next
			}
		}
	}
	time.Sleep(Delay)
	body, err := client.Get(fmt.Sprintf("%s%s?build=%s", baseURL, SettledPath, build))
	if err != nil {
		return c, err
	}
	var sl settledList
	if err := json.Unmarshal(body, &sl); err != nil {
		return c, fmt.Errorf("everloot settled: %w", err)
	}
	if c.SettledQuests, err = DecodeDelta(sl.Quest); err != nil {
		return c, fmt.Errorf("everloot settled quest: %w", err)
	}
	if c.SettledVendors, err = DecodeDelta(sl.Vendor); err != nil {
		return c, fmt.Errorf("everloot settled vendor: %w", err)
	}
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return c, err
	}
	b, _ := json.MarshalIndent(c, "", " ")
	return c, os.WriteFile(path, append(b, '\n'), 0o644)
}

// add decodes one record of the kind asked for; a record of another kind is skipped.
func (c *Corpus) add(kind string, raw json.RawMessage) error {
	var head struct {
		Kind string `json:"kind"`
	}
	if err := json.Unmarshal(raw, &head); err != nil {
		return fmt.Errorf("everloot corpus record: %w", err)
	}
	if head.Kind != kind {
		return nil
	}
	if kind == KindQuestRec {
		var r QuestRecord
		if err := json.Unmarshal(raw, &r); err != nil {
			return fmt.Errorf("everloot quest record: %w", err)
		}
		c.Quests = append(c.Quests, r)
		return nil
	}
	switch kind {
	case KindTrainerRec:
		var r TrainerRecord
		if err := json.Unmarshal(raw, &r); err != nil {
			return fmt.Errorf("everloot trainer record: %w", err)
		}
		c.Trainers = append(c.Trainers, r)
	case KindLootRec:
		var r LootRecord
		if err := json.Unmarshal(raw, &r); err != nil {
			return fmt.Errorf("everloot loot record: %w", err)
		}
		c.Loots = append(c.Loots, r)
	case KindEncounterRec:
		var r EncounterRecord
		if err := json.Unmarshal(raw, &r); err != nil {
			return fmt.Errorf("everloot encounter record: %w", err)
		}
		c.Encounters = append(c.Encounters, r)
	case KindEntranceRec:
		var r EntranceRecord
		if err := json.Unmarshal(raw, &r); err != nil {
			return fmt.Errorf("everloot entrance record: %w", err)
		}
		c.Entrances = append(c.Entrances, r)
	case KindVendorRec:
		var r VendorRecord
		if err := json.Unmarshal(raw, &r); err != nil {
			return fmt.Errorf("everloot vendor record: %w", err)
		}
		c.Vendors = append(c.Vendors, r)
	default:
		return fmt.Errorf("everloot corpus: unknown kind %q", kind)
	}
	return nil
}

// QuestObserved is one row of quests_observed. A field is null unless the quest is settled
// or at least MinReporters distinct installs reported that value.
type QuestObserved struct {
	QuestID        int32   `parquet:"quest_id"`
	Title          string  `parquet:"title"`
	Level          *int16  `parquet:"level,optional"`
	GiverNpcID     *int32  `parquet:"giver_npc_id,optional"`
	TurnInNpcID    *int32  `parquet:"turn_in_npc_id,optional"`
	Faction        *string `parquet:"faction,optional,dict"`
	Category       *string `parquet:"category,optional,dict"`
	SuggestedGroup *int16  `parquet:"suggested_group,optional"`
	Tag            *string `parquet:"tag,optional,dict"`
	Settled        bool    `parquet:"settled"`
	Reporters      int32   `parquet:"reporters"`
}

// vote counts the distinct installs behind each value of one field.
type vote[T comparable] map[T]map[string]struct{}

func (v vote[T]) add(val T, install string) {
	if v[val] == nil {
		v[val] = map[string]struct{}{}
	}
	v[val][install] = struct{}{}
}

// best returns the value most installs reported, the one that sorts first on a tie, and
// its install count.
func best[T comparable](v vote[T], less func(a, b T) bool) (T, int) {
	var top T
	n := 0
	for val, set := range v {
		if len(set) > n || (len(set) == n && less(val, top)) {
			top, n = val, len(set)
		}
	}
	return top, n
}

// pick returns the winning value when the quest is settled or enough installs agree.
func pick[T comparable](v vote[T], settled bool, less func(a, b T) bool) (T, bool) {
	val, n := best(v, less)
	return val, n > 0 && (settled || n >= MinReporters)
}

func ptr[T any](v T) *T { return &v }

type questVotes struct {
	installs map[string]struct{}
	title    vote[string]
	level    vote[int16]
	giver    vote[int32]
	turnIn   vote[int32]
	faction  vote[string]
	category vote[string]
	group    vote[int16]
	tag      vote[string]
	rewards  map[int32]map[string]struct{}
}

func groupQuests(recs []QuestRecord) map[int32]*questVotes {
	out := map[int32]*questVotes{}
	for _, r := range recs {
		if r.QuestID <= 0 || r.InstallID == "" {
			continue
		}
		q := out[r.QuestID]
		if q == nil {
			q = &questVotes{installs: map[string]struct{}{}, title: vote[string]{}, level: vote[int16]{}, giver: vote[int32]{},
				turnIn: vote[int32]{}, faction: vote[string]{}, category: vote[string]{}, group: vote[int16]{}, tag: vote[string]{},
				rewards: map[int32]map[string]struct{}{}}
			out[r.QuestID] = q
		}
		q.installs[r.InstallID] = struct{}{}
		if r.Title != "" {
			q.title.add(r.Title, r.InstallID)
		}
		if r.Level != nil {
			q.level.add(*r.Level, r.InstallID)
		}
		if r.GiverNpcID > 0 {
			q.giver.add(r.GiverNpcID, r.InstallID)
		}
		if r.TurnInNpcID > 0 {
			q.turnIn.add(r.TurnInNpcID, r.InstallID)
		}
		if r.Faction != "" {
			q.faction.add(r.Faction, r.InstallID)
		}
		if r.Category != "" {
			q.category.add(r.Category, r.InstallID)
		}
		if r.Group != nil && *r.Group > 0 {
			q.group.add(*r.Group, r.InstallID)
		}
		if r.Tag != "" {
			q.tag.add(r.Tag, r.InstallID)
		}
		for _, it := range r.RewardItemIDs {
			if it > 0 {
				if q.rewards[it] == nil {
					q.rewards[it] = map[string]struct{}{}
				}
				q.rewards[it][r.InstallID] = struct{}{}
			}
		}
	}
	return out
}

func lessS(a, b string) bool  { return a < b }
func lessI32(a, b int32) bool { return a < b }
func lessI16(a, b int16) bool { return a < b }

// BuildQuestsObserved keeps a quest when it is settled (listed in SettledQuests) or at
// least MinReporters distinct installs reported its title. Each field then takes the value
// most installs reported (the smallest on a tie) and is null unless the quest is settled
// or MinReporters installs agree on that value. Rows are sorted by quest id.
func BuildQuestsObserved(c Corpus) []QuestObserved {
	settled := idSet(c.SettledQuests)
	out := []QuestObserved{}
	for id, q := range groupQuests(c.Quests) {
		st := settled[id]
		title, ok := pick(q.title, st, lessS)
		if !ok {
			continue
		}
		row := QuestObserved{QuestID: id, Title: title, Settled: st, Reporters: int32(len(q.installs))}
		if v, ok := pick(q.level, st, lessI16); ok {
			row.Level = ptr(v)
		}
		if v, ok := pick(q.giver, st, lessI32); ok {
			row.GiverNpcID = ptr(v)
		}
		if v, ok := pick(q.turnIn, st, lessI32); ok {
			row.TurnInNpcID = ptr(v)
		}
		if v, ok := pick(q.faction, st, lessS); ok {
			row.Faction = ptr(v)
		}
		if v, ok := pick(q.category, st, lessS); ok {
			row.Category = ptr(v)
		}
		if v, ok := pick(q.group, st, lessI16); ok {
			row.SuggestedGroup = ptr(v)
		}
		if v, ok := pick(q.tag, st, lessS); ok {
			row.Tag = ptr(v)
		}
		out = append(out, row)
	}
	slices.SortFunc(out, func(a, b QuestObserved) int { return int(a.QuestID - b.QuestID) })
	return out
}

func idSet(ids []int32) map[int32]bool {
	m := make(map[int32]bool, len(ids))
	for _, id := range ids {
		m[id] = true
	}
	return m
}

// BuildQuestRewards returns a `quest reward` loot row for each reward item of a kept quest
// (see BuildQuestsObserved) that the quest is settled for or MinReporters installs listed.
// reporters is the installs that listed the item; tier is settled or corroborated.
func BuildQuestRewards(c Corpus) []LootSource {
	settled := idSet(c.SettledQuests)
	kept := map[int32]bool{}
	for _, r := range BuildQuestsObserved(c) {
		kept[r.QuestID] = true
	}
	var asOf *string
	if c.DataAsOf != "" {
		asOf = &c.DataAsOf
	}
	out := []LootSource{}
	for id, q := range groupQuests(c.Quests) {
		if !kept[id] {
			continue
		}
		for item, set := range q.rewards {
			if !settled[id] && len(set) < MinReporters {
				continue
			}
			tier := TierCorroborated
			if settled[id] {
				tier = TierSettled
			}
			out = append(out, LootSource{ItemID: item, SourceKind: KindQuest, SourceID: id, Reporters: int32(len(set)), Tier: &tier, EverlootDate: asOf})
		}
	}
	SortLootSources(out)
	return out
}

// VendorSource is one row of vendor_sources: an item a vendor sells.
type VendorSource struct {
	NpcID       int32 `parquet:"npc_id"`
	ItemID      int32 `parquet:"item_id"`
	PriceCopper int64 `parquet:"price_copper"`
	Limited     *bool `parquet:"limited,optional"`
	Reporters   int32 `parquet:"reporters"`
}

// BuildVendorSources keeps an offer when the vendor is settled (listed in SettledVendors)
// or at least MinReporters distinct installs listed the item for that vendor. The price is
// the one most installs saw (the lowest on a tie); limited is true when any install saw a
// limited stock on the offer and null otherwise. Rows are sorted by npc and item.
func BuildVendorSources(c Corpus) []VendorSource {
	type key struct{ npc, item int32 }
	type offer struct {
		installs map[string]struct{}
		price    map[int64]map[string]struct{}
		limited  bool
	}
	offers := map[key]*offer{}
	for _, r := range c.Vendors {
		if r.VendorNpcID <= 0 || r.InstallID == "" {
			continue
		}
		for _, it := range r.Items {
			if it.ItemID <= 0 || it.PriceCopper < 0 {
				continue
			}
			k := key{r.VendorNpcID, it.ItemID}
			o := offers[k]
			if o == nil {
				o = &offer{installs: map[string]struct{}{}, price: map[int64]map[string]struct{}{}}
				offers[k] = o
			}
			o.installs[r.InstallID] = struct{}{}
			if o.price[it.PriceCopper] == nil {
				o.price[it.PriceCopper] = map[string]struct{}{}
			}
			o.price[it.PriceCopper][r.InstallID] = struct{}{}
			if it.LimitedStock != nil && *it.LimitedStock > 0 {
				o.limited = true
			}
		}
	}
	settled := idSet(c.SettledVendors)
	out := []VendorSource{}
	for k, o := range offers {
		if !settled[k.npc] && len(o.installs) < MinReporters {
			continue
		}
		price, _ := best(vote[int64](o.price), func(a, b int64) bool { return a < b })
		row := VendorSource{NpcID: k.npc, ItemID: k.item, PriceCopper: price, Reporters: int32(len(o.installs))}
		if o.limited {
			row.Limited = ptr(true)
		}
		out = append(out, row)
	}
	slices.SortFunc(out, func(a, b VendorSource) int {
		if a.NpcID != b.NpcID {
			return int(a.NpcID - b.NpcID)
		}
		return int(a.ItemID - b.ItemID)
	})
	return out
}

// BuildSoldBy returns a `sold by` loot row per vendor offer: source_id is the vendor npc,
// reporters the installs that listed it, tier settled or corroborated; rates are zero (the
// price is in vendor_sources).
func BuildSoldBy(c Corpus, offers []VendorSource) []LootSource {
	settled := idSet(c.SettledVendors)
	var asOf *string
	if c.DataAsOf != "" {
		asOf = &c.DataAsOf
	}
	out := make([]LootSource, 0, len(offers))
	for _, o := range offers {
		tier := TierCorroborated
		if settled[o.NpcID] {
			tier = TierSettled
		}
		out = append(out, LootSource{ItemID: o.ItemID, SourceKind: KindSoldBy, SourceID: o.NpcID, Reporters: o.Reporters, Tier: &tier, EverlootDate: asOf})
	}
	SortLootSources(out)
	return out
}

// MergeNewKeys appends the extra rows whose (item, kind, source) key base lacks and sorts
// the union, so a row never appears twice.
func MergeNewKeys(base, extra []LootSource) []LootSource {
	type key struct {
		item int32
		kind string
		src  int32
	}
	seen := make(map[key]bool, len(base))
	for _, r := range base {
		seen[key{r.ItemID, r.SourceKind, r.SourceID}] = true
	}
	for _, r := range extra {
		k := key{r.ItemID, r.SourceKind, r.SourceID}
		if !seen[k] {
			seen[k] = true
			base = append(base, r)
		}
	}
	SortLootSources(base)
	return base
}

// CorpusName is the manifest source name of the quest and vendor tables.
const CorpusName = "Everloot (https://everloot.app), observed quests and vendor offers"
