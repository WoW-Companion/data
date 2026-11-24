package everloot

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"
)

func p64(v int64) *int64 { return &v }

func i64(v int64) *int64 { return &v }

func TestBuildTrainerOffersCorroboration(t *testing.T) {
	c := Corpus{Trainers: []TrainerRecord{
		{InstallID: "a", TrainerNpcID: 7, Profession: "First Aid", Recipe: "Silk Bandage", RequiredRank: i32(150), CostCopper: i64(5000)},
		{InstallID: "b", TrainerNpcID: 7, Profession: "First Aid", Recipe: "Silk Bandage", RequiredRank: i32(150), CostCopper: i64(5000)},
		{InstallID: "a", TrainerNpcID: 7, Profession: "First Aid", Recipe: "Linen Bandage", CostCopper: i64(100)}, // one install: dropped
		{InstallID: "a", TrainerNpcID: 8, Profession: "Cooking", Recipe: "Roast", CostCopper: i64(10)},
		{InstallID: "b", TrainerNpcID: 8, Profession: "Cooking", Recipe: "Roast", CostCopper: i64(20)}, // two costs, no agreement: null
		{InstallID: "", TrainerNpcID: 9, Profession: "Cooking", Recipe: "X"},
	}}
	got := BuildTrainerOffers(c)
	if len(got) != 2 {
		t.Fatalf("got %+v", got)
	}
	if got[0].NpcID != 7 || *got[0].CostCopper != 5000 || *got[0].RequiredSkill != 150 || got[0].Reporters != 2 {
		t.Errorf("offer 0: %+v", got[0])
	}
	if got[1].CostCopper != nil || got[1].RequiredSkill != nil {
		t.Errorf("disagreeing cost must be null: %+v", got[1])
	}
}

func gather(inst string, obj, zone int32, kind, name string, item, n int32) LootRecord {
	return LootRecord{InstallID: inst, LootType: LootGather, SourceObjectID: obj, MapID: zone, GatherKind: kind, SourceObjectName: name, Items: []LootItem{{item, n}}}
}

func TestBuildGatherObs(t *testing.T) {
	c := Corpus{Loots: []LootRecord{
		gather("a", 1731, 1413, "ore", "Copper Vein", 2770, 1),
		gather("b", 1731, 1413, "ore", "Copper Vein", 2770, 3),
		gather("a", 1731, 1413, "ore", "Copper Vein", 2771, 1), // one install: no yield row
		gather("a", 5, 1413, "ore", "Lone", 9, 1),              // one install: no node
		{InstallID: "a", LootType: "kill"},
	}}
	nodes, yields := BuildGatherObs(c)
	want := []NodeObs{{ObjectID: 1731, ZoneID: 1413, Name: "Copper Vein", GatherKind: "ore", Windows: 3, Reporters: 2}}
	if !reflect.DeepEqual(nodes, want) {
		t.Errorf("nodes %+v", nodes)
	}
	if !reflect.DeepEqual(yields, []NodeYield{{ObjectID: 1731, ItemID: 2770, Windows: 2, Reporters: 2, MinCount: 1, MaxCount: 3}}) {
		t.Errorf("yields %+v", yields)
	}
}

func TestBuildFishing(t *testing.T) {
	fish := func(inst string, item int32) LootRecord {
		return LootRecord{InstallID: inst, LootType: LootFishing, MapID: 1413, Items: []LootItem{{item, 1}}}
	}
	c := Corpus{Loots: []LootRecord{fish("a", 6291), fish("b", 6291), fish("a", 6303), fish("b", 6291), {InstallID: "a", GatherKind: "fishing", MapID: 1413}}}
	got := BuildFishing(c)
	if len(got) != 1 || got[0].ItemID != 6291 || got[0].Casts != 5 || got[0].Catches != 3 || got[0].Reporters != 2 || got[0].Rate != 0.6 {
		t.Errorf("got %+v", got)
	}
}

func TestBuildDisenchantYields(t *testing.T) {
	d := Disenchant{Bands: []DisenchantBand{
		{BandID: 5, ItemClass: "armor", Quality: "uncommon", MinLevel: 16, MaxLevel: 20, State: "rated", Windows: 55, Installs: 4,
			Materials: []DisenchantMaterial{{ItemID: 10939, Chance: 0.27, PerDisenchant: 0.3, Min: 1, Max: 2}, {ItemID: 10940, Chance: 0.63, PerDisenchant: 1.6, Min: 2, Max: 3}}},
		{BandID: 6, ItemClass: "weapon", Quality: "rare", State: "none"},
	}}
	got := BuildDisenchantYields(d)
	if len(got) != 2 || got[0].MaterialID != 10939 || got[1].MaxCount != 3 || got[1].MinItemLevel != 16 || got[1].Quality != "uncommon" {
		t.Errorf("got %+v", got)
	}
}

// A fake server stands in for Everloot: the fetch reads trainer records and the rollup, and
// leaves the 50 loot pages alone unless asked.
func TestFetchReadsTrainersAndDisenchantWithoutLoot(t *testing.T) {
	Delay = time.Millisecond
	kinds := map[string]int{}
	rollup := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case SettledPath:
			json.NewEncoder(w).Encode(map[string]any{"quest": []int64{}, "vendor": []int64{}})
		case DisenchantPath:
			rollup++
			json.NewEncoder(w).Encode(map[string]any{"dataAsOf": "d", "build": "b1", "bands": []DisenchantBand{{BandID: 4, ItemClass: "armor", Quality: "uncommon", MinLevel: 5, MaxLevel: 15, State: "rated", Materials: []DisenchantMaterial{{ItemID: 10940, Chance: 0.8}}}}})
		default:
			k := r.URL.Query().Get("kinds")
			kinds[k]++
			var recs []map[string]any
			if k == KindTrainerRec {
				recs = []map[string]any{{"kind": "trainer_seen", "installId": "a", "profession": "Cooking", "recipe": "Roast", "requiredRank": 1, "costCopper": 10, "trainerNpcId": 8}}
			}
			json.NewEncoder(w).Encode(map[string]any{"dataAsOf": "d", "records": recs, "next": nil})
		}
	}))
	defer srv.Close()
	dir := t.TempDir()
	c, err := FetchCorpus(srv.URL, dir, "now", "b1")
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Trainers) != 1 || c.Trainers[0].TrainerNpcID != 8 || *c.Trainers[0].CostCopper != 10 {
		t.Errorf("trainers %+v", c.Trainers)
	}
	if kinds[KindLootRec] != 0 {
		t.Errorf("loot was fetched without the opt-in: %v", kinds)
	}
	d, err := FetchDisenchant(srv.URL, dir, "now")
	if err != nil || len(d.Bands) != 1 || d.Bands[0].Materials[0].ItemID != 10940 || d.Retrieved != "now" {
		t.Fatalf("%+v %v", d, err)
	}
	if _, err := FetchDisenchant(srv.URL, dir, "later"); err != nil || rollup != 1 {
		t.Errorf("cache not kept: %d requests, %v", rollup, err)
	}
	if got, _ := LoadDisenchant(dir + "/none.json"); len(got.Bands) != 0 {
		t.Error("a missing file must be an empty rollup")
	}
}

func TestFetchLootIsOptIn(t *testing.T) {
	Delay = time.Millisecond
	FetchLoot = true
	defer func() { FetchLoot = false }()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == SettledPath {
			json.NewEncoder(w).Encode(map[string]any{"quest": []int64{}, "vendor": []int64{}})
			return
		}
		var recs []map[string]any
		if r.URL.Query().Get("kinds") == KindLootRec {
			recs = []map[string]any{{"kind": "loot", "installId": "a", "lootType": "gather", "sourceObjectId": 4, "mapId": 1413, "gatherKind": "herb", "items": []map[string]any{{"itemId": 2447, "count": 1}}}}
		}
		json.NewEncoder(w).Encode(map[string]any{"dataAsOf": "d", "records": recs, "next": nil})
	}))
	defer srv.Close()
	c, err := FetchCorpus(srv.URL, t.TempDir(), "now", "b1")
	if err != nil || len(c.Loots) != 1 || c.Loots[0].Items[0].ItemID != 2447 {
		t.Fatalf("%+v %v", c.Loots, err)
	}
}
