package everloot

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func i16(v int16) *int16 { return &v }
func i32(v int32) *int32 { return &v }

func testCorpus() Corpus {
	return Corpus{DataAsOf: "2026-01-02T00:00:00Z", SettledQuests: []int32{7000}, SettledVendors: []int32{600},
		Quests: []QuestRecord{
			{InstallID: "a", QuestID: 91001, Title: "Test Quest", Level: i16(5), GiverNpcID: 501, RewardItemIDs: []int32{801, 802}, Faction: "horde"},
			{InstallID: "b", QuestID: 91001, Title: "Test Quest", Level: i16(5), GiverNpcID: 502, RewardItemIDs: []int32{801}},
			{InstallID: "b", QuestID: 91001, Title: "Test Quest", Level: i16(5)}, // same install twice counts once
			{InstallID: "a", QuestID: 91002, Title: "Lone Quest", RewardItemIDs: []int32{803}},
			{InstallID: "c", QuestID: 7000, Title: "Settled Quest", GiverNpcID: 503, RewardItemIDs: []int32{804}, Group: i16(5), Tag: "group"},
			{InstallID: "d", QuestID: 91003, Title: "Spelling A"},
			{InstallID: "e", QuestID: 91003, Title: "Spelling B"},
			{InstallID: "f", QuestID: 91003, Title: "Spelling B"},
			{InstallID: "", QuestID: 91004, Title: "No Install"},
		},
		Vendors: []VendorRecord{
			{InstallID: "a", VendorNpcID: 601, Items: []VendorItem{{ItemID: 11, PriceCopper: 50}, {ItemID: 12, PriceCopper: 70, LimitedStock: i32(1)}}},
			{InstallID: "b", VendorNpcID: 601, Items: []VendorItem{{ItemID: 11, PriceCopper: 50}, {ItemID: 12, PriceCopper: 70}}},
			{InstallID: "c", VendorNpcID: 601, Items: []VendorItem{{ItemID: 11, PriceCopper: 60}, {ItemID: 13, PriceCopper: 9}}},
			{InstallID: "a", VendorNpcID: 600, Items: []VendorItem{{ItemID: 14, PriceCopper: 1}}},
		}}
}

func TestBuildQuestsObserved(t *testing.T) {
	got := BuildQuestsObserved(testCorpus())
	if len(got) != 3 || got[0].QuestID != 7000 || got[1].QuestID != 91001 || got[2].QuestID != 91003 {
		t.Fatalf("kept quests: %+v", got)
	}
	s := got[0]
	if !s.Settled || s.GiverNpcID == nil || *s.GiverNpcID != 503 || s.SuggestedGroup == nil || *s.SuggestedGroup != 5 || *s.Tag != "group" {
		t.Errorf("settled quest keeps single-install fields: %+v", s)
	}
	c := got[1]
	if c.Settled || c.Reporters != 2 || c.Level == nil || *c.Level != 5 || c.TurnInNpcID != nil {
		t.Errorf("corroborated quest: %+v", c)
	}
	if c.GiverNpcID != nil || c.Faction != nil { // two givers disagree; one install said horde
		t.Errorf("uncorroborated fields must be null: %+v", c)
	}
	if got[2].Title != "Spelling B" {
		t.Errorf("most reported title wins: %+v", got[2])
	}
}

func TestBuildQuestRewards(t *testing.T) {
	got := BuildQuestRewards(testCorpus())
	var keys []string
	for _, r := range got {
		keys = append(keys, fmt.Sprintf("%d/%s/%d/%d/%s", r.ItemID, r.SourceKind, r.SourceID, r.Reporters, *r.Tier))
	}
	want := []string{"801/quest reward/91001/2/corroborated", "804/quest reward/7000/1/settled"}
	if !reflect.DeepEqual(keys, want) {
		t.Fatalf("got %v want %v", keys, want)
	}
}

func TestBuildVendorSources(t *testing.T) {
	c := testCorpus()
	got := BuildVendorSources(c)
	var keys []string
	for _, r := range got {
		keys = append(keys, fmt.Sprintf("%d/%d/%d/%d", r.NpcID, r.ItemID, r.PriceCopper, r.Reporters))
	}
	// item 11: price 50 seen by two installs beats 60 by one; 13 has one reporter; settled vendor 600 keeps its single offer.
	want := []string{"600/14/1/1", "601/11/50/3", "601/12/70/2"}
	if !reflect.DeepEqual(keys, want) {
		t.Fatalf("got %v want %v", keys, want)
	}
	if got[2].Limited == nil || !*got[2].Limited || got[1].Limited != nil {
		t.Errorf("limited: %+v", got)
	}
	sold := BuildSoldBy(c, got)
	if len(sold) != 3 || sold[0].SourceKind != KindSoldBy || *sold[0].Tier != TierCorroborated || *sold[2].Tier != TierSettled || sold[1].Rate != 0 {
		t.Errorf("sold by rows: %+v", sold)
	}
}

func TestMergeNewKeysSkipsExisting(t *testing.T) {
	base := []LootSource{{ItemID: 1, SourceKind: KindQuest, SourceID: 9, Reporters: 4}}
	got := MergeNewKeys(base, []LootSource{{ItemID: 1, SourceKind: KindQuest, SourceID: 9, Reporters: 1}, {ItemID: 1, SourceKind: KindSoldBy, SourceID: 9}})
	if len(got) != 2 || got[0].SourceKind != KindQuest || got[0].Reporters != 4 {
		t.Errorf("got %+v", got)
	}
}

func TestDecodeDelta(t *testing.T) {
	got, err := DecodeDelta([]int64{3, 4, 1})
	if err != nil || !reflect.DeepEqual(got, []int32{3, 7, 8}) {
		t.Errorf("got %v %v", got, err)
	}
	if _, err := DecodeDelta([]int64{3, -1}); err == nil {
		t.Error("a negative gap must fail")
	}
	if got, err := DecodeDelta(nil); err != nil || len(got) != 0 {
		t.Errorf("empty list: %v %v", got, err)
	}
}

func TestFetchCorpusFollowsNextAndCaches(t *testing.T) {
	Delay = time.Millisecond
	hits := 0
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		q := r.URL.Query()
		switch r.URL.Path {
		case SettledPath:
			if q.Get("build") != "b1" {
				http.Error(w, "build", 400)
			}
			json.NewEncoder(w).Encode(map[string]any{"quest": []int64{5, 2}, "vendor": []int64{}})
		case CorpusPath:
			kind := q.Get("kinds")
			p := map[string]any{"dataAsOf": "d", "attribution": Attribution{Credit: Credit}, "next": nil}
			if kind == KindQuestRec {
				p["records"] = []map[string]any{{"kind": "quest", "installId": "a", "questId": 91001, "title": "T"}}
				if q.Get("offset") == "0" {
					p["next"] = srv.URL + CorpusPath + "?kinds=quest&limit=500&offset=1"
				} else {
					p["records"] = []map[string]any{{"kind": "quest", "installId": "b", "questId": 91001, "title": "T"}, {"kind": "loot"}}
				}
			} else {
				p["records"] = []map[string]any{{"kind": "vendor", "installId": "a", "vendorNpcId": 601, "items": []map[string]any{{"itemId": 1, "priceCopper": 5}}}}
			}
			json.NewEncoder(w).Encode(p)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	dir := t.TempDir()
	c, err := FetchCorpus(srv.URL, dir, "now", "b1")
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Quests) != 2 || len(c.Vendors) != 1 || !reflect.DeepEqual(c.SettledQuests, []int32{5, 7}) || c.DataAsOf != "d" {
		t.Fatalf("corpus: %+v", c)
	}
	n := hits
	again, err := FetchCorpus(srv.URL, dir, "later", "b1")
	if err != nil || hits != n || again.Retrieved != "now" || len(again.Quests) != 2 {
		t.Errorf("cache not reused: hits %d→%d %v", n, hits, err)
	}
	if got, err := LoadCorpus(filepath.Join(dir, "none.json")); err != nil || len(got.Quests) != 0 {
		t.Errorf("a missing cache is an empty corpus: %v", err)
	}
}

func TestFetchCorpusStopsOnError(t *testing.T) {
	Delay = time.Millisecond
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "no", 404) }))
	defer srv.Close()
	dir := t.TempDir()
	if _, err := FetchCorpus(srv.URL, dir, "now", "b1"); err == nil {
		t.Fatal("an error must stop the fetch")
	}
	if _, err := os.Stat(filepath.Join(dir, CorpusFile)); err == nil {
		t.Error("no cache after a failed fetch")
	}
}
