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
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return s, err
	}
	b, _ := json.MarshalIndent(s, "", " ")
	return s, os.WriteFile(path, append(b, '\n'), 0o644)
}

func getPage(c *http.Client, u string) (page, error) {
	var p page
	resp, err := c.Get(u)
	if err != nil {
		return p, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return p, fmt.Errorf("%s: HTTP %d (Retry-After %q)", u, resp.StatusCode, resp.Header.Get("Retry-After"))
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
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
}

// BuildLootSources keeps the public rates that name a creature or object source and
// returns them sorted by item, kind and source.
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
	slices.SortStableFunc(out, func(a, b LootSource) int {
		if a.ItemID != b.ItemID {
			return int(a.ItemID - b.ItemID)
		}
		if a.SourceKind != b.SourceKind {
			return strings.Compare(a.SourceKind, b.SourceKind)
		}
		return int(a.SourceID - b.SourceID)
	})
	return out
}
