package everloot

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"
)

func TestBuildLootSources(t *testing.T) {
	lo := 0.1
	s := Snapshot{DataAsOf: "2026-11-20", Rates: []Rate{
		{ItemID: 9, SourceObjectID: 4, Kills: 3, Drops: 1, Rate: 0.33, Public: true},
		{ItemID: 9, SourceNpcID: 70, Kills: 10, Drops: 2, Rate: 0.2, RateLow: &lo, Reporters: 3, Tier: "verified", Public: true},
		{ItemID: 5, SourceNpcID: 1, Public: false},
		{ItemID: 5, Public: true},
		{ItemID: 0, SourceNpcID: 1, Public: true},
		{ItemID: 2, SourceNpcID: 8, Rate: 1, Public: true},
	}}
	got := BuildLootSources(s)
	var keys []string
	for _, r := range got {
		keys = append(keys, fmt.Sprintf("%d/%s/%d", r.ItemID, r.SourceKind, r.SourceID))
	}
	if want := []string{"2/creature/8", "9/creature/70", "9/object/4"}; !reflect.DeepEqual(keys, want) {
		t.Fatalf("got %v want %v", keys, want)
	}
	r := got[1]
	if r.Rate != 0.2 || r.Kills != 10 || r.RateLow != &lo || r.RateHigh != nil || *r.Tier != "verified" || *r.EverlootDate != "2026-11-20" {
		t.Errorf("row not kept as given: %+v", r)
	}
	if got[0].Tier != nil {
		t.Errorf("absent tier must be null")
	}
}

func TestFetchFollowsNextAndCaches(t *testing.T) {
	Delay = time.Millisecond
	var srv *httptest.Server
	hits := 0
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		p := map[string]any{"dataAsOf": "d", "attribution": Attribution{Credit: Credit}}
		if r.URL.Query().Get("offset") == "0" {
			p["rates"] = []Rate{{ItemID: 1, SourceNpcID: 2, Public: true}}
			p["next"] = srv.URL + DropsPath + "?offset=1&limit=500"
		} else {
			p["rates"] = []Rate{{ItemID: 3, SourceNpcID: 4, Public: true}}
			p["next"] = nil
		}
		json.NewEncoder(w).Encode(p)
	}))
	defer srv.Close()
	dir := t.TempDir()
	s, err := Fetch(srv.URL, dir, "now")
	if err != nil || len(s.Rates) != 2 || s.DataAsOf != "d" || s.Attribution.Credit != Credit {
		t.Fatalf("%+v, %v", s, err)
	}
	if _, err := Fetch(srv.URL, dir, "later"); err != nil || hits != 2 {
		t.Fatalf("second fetch must use the cache: hits=%d err=%v", hits, err)
	}
	if l, err := Load(dir + "/" + CacheFile); err != nil || l.Retrieved != "now" || len(l.Rates) != 2 {
		t.Fatalf("%+v %v", l, err)
	}
}
