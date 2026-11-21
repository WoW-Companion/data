// Package everloot fetches Everloot's corroborated drop rates (CC BY-SA 4.0) into a local
// cache and turns them into loot_sources rows.
//
// Terms are recorded in docs/sources.md. Rates are kept exactly as Everloot gives them.
package everloot

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

const (
	BaseURL   = "https://everloot.app"
	DropsPath = "/api/rollups/drops"
	CacheFile = "drops.json"
	EdgesPath = "/api/edges"
	RelReward = "reward_from"
	// Credit is the attribution wording from Everloot's data policy.
	Credit  = "Data from Everloot (https://everloot.app), CC BY-SA 4.0."
	License = "CC BY-SA 4.0"
	PageLen = 500 // the cap without an API key
)

// Delay is the pause between two page requests.
var Delay = 2 * time.Second

// Rate is one rates[] row of the drops answer.
type Rate struct {
	SourceNpcID    int32    `json:"sourceNpcId,omitempty"`
	SourceObjectID int32    `json:"sourceObjectId,omitempty"`
	ItemID         int32    `json:"itemId"`
	Kills          int32    `json:"kills"`
	Drops          int32    `json:"drops"`
	Rate           float64  `json:"rate"`
	RateLow        *float64 `json:"rateLow,omitempty"`
	RateHigh       *float64 `json:"rateHigh,omitempty"`
	Reporters      int32    `json:"reporters"`
	Public         bool     `json:"public"`
	Tier           string   `json:"tier,omitempty"`
}

// Edge is one edges[] row of the relations answer.
type Edge struct {
	FromKind string `json:"fromKind"`
	FromID   int32  `json:"fromId"`
	Rel      string `json:"rel"`
	ToKind   string `json:"toKind"`
	ToID     int32  `json:"toId"`
	Evidence string `json:"evidence,omitempty"`
	Installs int32  `json:"installs"`
}

type edgePage struct {
	Edges []Edge  `json:"edges"`
	Next  *string `json:"next"`
}

// Required record fields. A field absent from every record of a non-empty page is a
// renamed field, not data, and stops the fetch.
var (
	dropFields = []string{"itemId", "kills", "drops", "rate", "reporters", "public"}
	edgeFields = []string{"fromKind", "fromId", "rel", "toKind", "toId", "installs"}
)

// RequireFields checks that each field appears in at least one record of the array
// body[key]. An empty array passes: there is nothing to check.
func RequireFields(table, key string, fields []string, body []byte) error {
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(body, &doc); err != nil {
		return fmt.Errorf("%s: %w", table, err)
	}
	raw, ok := doc[key]
	if !ok {
		return fmt.Errorf("%s: required field %q missing from the answer", table, key)
	}
	var recs []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &recs); err != nil {
		return fmt.Errorf("%s: %q: %w", table, key, err)
	}
	if len(recs) == 0 {
		return nil
	}
	for _, f := range fields {
		found := false
		for _, r := range recs {
			if _, ok := r[f]; ok {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("%s: required field %q missing from every record", table, f)
		}
	}
	return nil
}

// Attribution is the attribution object every answer carries.
type Attribution struct {
	Licence    string `json:"licence"`
	LicenceURL string `json:"licenceUrl"`
	Credit     string `json:"credit"`
	Source     string `json:"source"`
	Terms      string `json:"terms"`
}

// Snapshot is the cached union of every page.
type Snapshot struct {
	Retrieved   string      `json:"retrieved"`
	DataAsOf    string      `json:"dataAsOf"`
	Attribution Attribution `json:"attribution"`
	Rates       []Rate      `json:"rates"`
	// QuestRewards are the item-to-quest "reward_from" edges from /api/edges.
	QuestRewards []Edge `json:"questRewards,omitempty"`
}

type page struct {
	DataAsOf    string      `json:"dataAsOf"`
	Attribution Attribution `json:"attribution"`
	Rates       []Rate      `json:"rates"`
	Next        *string     `json:"next"`
}

// Fetch downloads every page of drop rates and writes cacheDir/drops.json. An existing
// cache file is kept as is.
func Fetch(baseURL, cacheDir, retrieved string) (Snapshot, error) {
	path := filepath.Join(cacheDir, CacheFile)
	if b, err := os.ReadFile(path); err == nil {
		var s Snapshot
		return s, json.Unmarshal(b, &s)
	}
	s := Snapshot{Retrieved: retrieved}
	client := &http.Client{Timeout: 120 * time.Second}
	u := fmt.Sprintf("%s%s?offset=0&limit=%d", baseURL, DropsPath, PageLen)
	for first := true; u != ""; first = false {
		if !first {
			time.Sleep(Delay)
		}
		p, err := getPage(client, u)
		if err != nil {
			return s, err
		}
		s.Rates = append(s.Rates, p.Rates...)
		s.DataAsOf, s.Attribution = p.DataAsOf, p.Attribution
		u = ""
		if p.Next != nil {
			u = *p.Next
		}
	}
	u = fmt.Sprintf("%s%s?rel=%s&offset=0&limit=%d", baseURL, EdgesPath, RelReward, PageLen)
	for u != "" {
		time.Sleep(Delay)
		p, err := getEdges(client, u)
		if err != nil {
			return s, err
		}
		s.QuestRewards = append(s.QuestRewards, p.Edges...)
		u = ""
		if p.Next != nil {
			u = *p.Next
		}
	}
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return s, err
	}
	b, _ := json.MarshalIndent(s, "", " ")
	return s, os.WriteFile(path, append(b, '\n'), 0o644)
}

func getBody(c *http.Client, u string) ([]byte, error) {
	resp, err := c.Get(u)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: HTTP %d (Retry-After %q)", u, resp.StatusCode, resp.Header.Get("Retry-After"))
	}
	return io.ReadAll(resp.Body)
}

func getPage(c *http.Client, u string) (page, error) {
	var p page
	body, err := getBody(c, u)
	if err != nil {
		return p, err
	}
	if err := RequireFields("everloot drops", "rates", dropFields, body); err != nil {
		return p, err
	}
	return p, json.Unmarshal(body, &p)
}

func getEdges(c *http.Client, u string) (edgePage, error) {
	var p edgePage
	body, err := getBody(c, u)
	if err != nil {
		return p, err
	}
	if err := RequireFields("everloot edges", "edges", edgeFields, body); err != nil {
		return p, err
	}
	return p, json.Unmarshal(body, &p)
}

// Load reads a cached snapshot.
func Load(path string) (Snapshot, error) {
	var s Snapshot
	b, err := os.ReadFile(path)
	if err != nil {
		return s, err
	}
	return s, json.Unmarshal(b, &s)
}

// Source kinds of a loot_sources row.
const (
	KindCreature = "creature"
	KindObject   = "object"
	KindQuest    = "quest reward"
	KindCrafted  = "crafted"
)

// LootSource is one row of the loot_sources table. Counts and rates are Everloot's own.
type LootSource struct {
	ItemID       int32    `parquet:"item_id"`
	SourceKind   string   `parquet:"source_kind"`
	SourceID     int32    `parquet:"source_id"`
	Kills        int32    `parquet:"kills"`
	Drops        int32    `parquet:"drops"`
	Rate         float64  `parquet:"rate"`
	RateLow      *float64 `parquet:"rate_low,optional"`
	RateHigh     *float64 `parquet:"rate_high,optional"`
	Reporters    int32    `parquet:"reporters"`
	Tier         *string  `parquet:"tier,optional"`
	EverlootDate *string  `parquet:"everloot_data_as_of,optional"`
	// Profession and LearnSkill are set on crafted rows only: the recipe's skill line
	// name and the skill needed to learn it. SourceID is the recipe spell id there.
	Profession *string `parquet:"profession,optional"`
	LearnSkill *int16  `parquet:"learn_skill,optional"`
}

// BuildLootSources keeps the public rates that name a creature or object source, plus a
// `quest reward` row per item-to-quest reward edge (kills, drops and rate are zero there:
// Everloot counts installs that saw the reward, not loot windows; installs go in
// reporters, the edge's evidence in tier). Rows are sorted by item, kind and source.
func BuildLootSources(s Snapshot) []LootSource {
	out := make([]LootSource, 0, len(s.Rates))
	var asOf *string
	if s.DataAsOf != "" {
		asOf = &s.DataAsOf
	}
	for _, r := range s.Rates {
		if !r.Public || r.ItemID <= 0 {
			continue
		}
		l := LootSource{
			ItemID: r.ItemID, Kills: r.Kills, Drops: r.Drops, Rate: r.Rate,
			RateLow: r.RateLow, RateHigh: r.RateHigh, Reporters: r.Reporters, EverlootDate: asOf,
		}
		switch {
		case r.SourceNpcID > 0:
			l.SourceKind, l.SourceID = KindCreature, r.SourceNpcID
		case r.SourceObjectID > 0:
			l.SourceKind, l.SourceID = KindObject, r.SourceObjectID
		default:
			continue
		}
		if r.Tier != "" {
			t := r.Tier
			l.Tier = &t
		}
		out = append(out, l)
	}
	for _, e := range s.QuestRewards {
		if e.Rel != RelReward || e.FromKind != "item" || e.ToKind != "quest" || e.FromID <= 0 || e.ToID <= 0 {
			continue
		}
		l := LootSource{ItemID: e.FromID, SourceKind: KindQuest, SourceID: e.ToID, Reporters: e.Installs, EverlootDate: asOf}
		if e.Evidence != "" {
			ev := e.Evidence
			l.Tier = &ev
		}
		out = append(out, l)
	}
	SortLootSources(out)
	return out
}

// SortLootSources orders rows by item, kind and source id.
func SortLootSources(out []LootSource) {
	slices.SortStableFunc(out, func(a, b LootSource) int {
		if a.ItemID != b.ItemID {
			return int(a.ItemID - b.ItemID)
		}
		if a.SourceKind != b.SourceKind {
			return strings.Compare(a.SourceKind, b.SourceKind)
		}
		return int(a.SourceID - b.SourceID)
	})
}
