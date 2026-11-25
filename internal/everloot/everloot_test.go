package everloot

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
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
		if r.URL.Path == EdgesPath {
			e := Edge{FromKind: "item", FromID: 1, Rel: RelReward, ToKind: "quest", ToID: 9, Installs: 2}
			if r.URL.Query().Get("rel") == RelLocated {
				e = Edge{FromKind: "npc", FromID: 2, Rel: RelLocated, ToKind: "zone", ToID: 1413, Installs: 3}
			}
			json.NewEncoder(w).Encode(map[string]any{"edges": []Edge{e}, "next": nil})
			return
		}
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
	if err != nil || len(s.Rates) != 2 || len(s.QuestRewards) != 1 || len(s.NpcZones) != 1 || s.DataAsOf != "d" || s.Attribution.Credit != Credit {
		t.Fatalf("%+v, %v", s, err)
	}
	if _, err := Fetch(srv.URL, dir, "later"); err != nil || hits != 4 {
		t.Fatalf("second fetch must use the cache: hits=%d err=%v", hits, err)
	}
	if l, err := Load(dir + "/" + CacheFile); err != nil || l.Retrieved != "now" || len(l.Rates) != 2 {
		t.Fatalf("%+v %v", l, err)
	}
}

func TestRequireFields(t *testing.T) {
	cases := []struct {
		name, body, wantErr string
	}{
		{"present in some records", `{"rates":[{"itemId":1,"kills":0,"drops":0,"rate":0,"reporters":0,"public":true},{"itemId":2}]}`, ""},
		{"empty page", `{"rates":[]}`, ""},
		{"renamed field", `{"rates":[{"itemId":1,"kills":0,"drops":0,"rate":0,"reporters":0,"isPublic":true}]}`, `everloot drops: required field "public" missing from every record`},
		{"no array", `{"edges":[]}`, `required field "rates" missing`},
	}
	for _, c := range cases {
		err := RequireFields("everloot drops", "rates", dropFields, []byte(c.body))
		if (err == nil) != (c.wantErr == "") || (err != nil && !strings.Contains(err.Error(), c.wantErr)) {
			t.Errorf("%s: err %v, want %q", c.name, err, c.wantErr)
		}
	}
	if err := RequireFields("everloot edges", "edges", edgeFields, []byte(`{"edges":[{"fromKind":"item"}]}`)); err == nil || !strings.Contains(err.Error(), `"fromId"`) {
		t.Errorf("edges: %v", err)
	}
}

func TestBuildLootSourcesQuestRewards(t *testing.T) {
	s := Snapshot{DataAsOf: "d", QuestRewards: []Edge{
		{FromKind: "item", FromID: 7, Rel: RelReward, ToKind: "quest", ToID: 50, Evidence: "observed", Installs: 4},
		{FromKind: "item", FromID: 7, Rel: "dropped_by", ToKind: "quest", ToID: 51},
		{FromKind: "item", FromID: 0, Rel: RelReward, ToKind: "quest", ToID: 52},
	}}
	got := BuildLootSources(s)
	if len(got) != 1 || got[0].SourceKind != KindQuest || got[0].SourceID != 50 || got[0].ItemID != 7 || got[0].Reporters != 4 || *got[0].Tier != "observed" {
		t.Fatalf("%+v", got)
	}
}

func TestBuildNpcZones(t *testing.T) {
	loc := func(npc, zone, installs int32) Edge {
		return Edge{FromKind: "npc", FromID: npc, Rel: RelLocated, ToKind: "zone", ToID: zone, Installs: installs}
	}
	bad := loc(9, 5, 1)
	bad.Rel = "drops_in"
	obj := loc(8, 5, 1)
	obj.FromKind = "object"
	got := BuildNpcZones([]Edge{loc(1, 20, 3), loc(1, 10, 3), loc(1, 30, 2), loc(2, 7, 1), loc(2, 8, 4), loc(3, 0, 1), loc(0, 4, 1), bad, obj})
	want := map[int32]NpcZone{1: {10, 3}, 2: {8, 4}}
	if len(got) != len(want) || got[1] != want[1] || got[2] != want[2] {
		t.Errorf("got %+v want %+v", got, want)
	}
}

func TestBuildNpcNames(t *testing.T) {
	rates := []Rate{
		{SourceNpcID: 1, SourceName: "Zed", SourceNameTier: "reported", Public: true},
		{SourceNpcID: 1, SourceName: "Alf", SourceNameTier: "confirmed", Public: true},
		{SourceNpcID: 2, SourceName: "Beta", Public: true},
		{SourceNpcID: 2, SourceName: "Alpha", Public: true},
		{SourceNpcID: 3, SourceName: "Hidden"},
		{SourceNpcID: 4, Public: true},
		{SourceObjectID: 5, SourceName: "Node", Public: true},
	}
	got := BuildNpcNames(rates)
	if len(got) != 2 || got[1] != "Alf" || got[2] != "Alpha" {
		t.Errorf("best tier, then smallest; public named creature rows only: %v", got)
	}
}

// A French Everloot name is dropped, and the creature keeps its English one when another
// row has it; with only the French name it gets no name at all.
func TestBuildNpcNamesEnglishOnly(t *testing.T) {
	before := DroppedNames.Load()
	got := BuildNpcNames([]Rate{
		{SourceNpcID: 1, SourceName: "Contremaître de la KapitalRisk", SourceNameTier: "confirmed", Public: true},
		{SourceNpcID: 1, SourceName: "KapitalRisk Foreman", SourceNameTier: "reported", Public: true},
		{SourceNpcID: 2, SourceName: "Géomètre de la KapitalRisk", Public: true},
	})
	if got[1] != "KapitalRisk Foreman" {
		t.Errorf("npc 1 = %q", got[1])
	}
	if _, ok := got[2]; ok {
		t.Errorf("npc 2 should have no name: %q", got[2])
	}
	if d := DroppedNames.Load() - before; d != 2 {
		t.Errorf("dropped %d, want 2", d)
	}
}
