package everloot

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"
)

func TestBuildEncounterObsNeedsTwoInstallsAndAKill(t *testing.T) {
	c := Corpus{Encounters: []EncounterRecord{
		{InstallID: "a", EncounterID: 1, InstanceMapID: 36, Success: true},
		{InstallID: "a", EncounterID: 1, InstanceMapID: 36, Success: true}, // same install twice counts once
		{InstallID: "b", EncounterID: 1, InstanceMapID: 36, Success: true},
		{InstallID: "a", EncounterID: 2, InstanceMapID: 36, Success: true},
		{InstallID: "a", EncounterID: 3, InstanceMapID: 36, Success: true},
		{InstallID: "b", EncounterID: 3, InstanceMapID: 36, Success: false}, // a wipe is not a kill
		{InstallID: "", EncounterID: 4, InstanceMapID: 36, Success: true},
	}}
	got := BuildEncounterObs(c)
	if want := map[EncounterKey]int32{{36, 1}: 2}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %v want %v", got, want)
	}
}

func TestBuildEntranceZones(t *testing.T) {
	c := Corpus{Entrances: []EntranceRecord{
		{InstallID: "a", InstanceMapID: 389, MapID: 1454}, {InstallID: "b", InstanceMapID: 389, MapID: 1454},
		{InstallID: "a", InstanceMapID: 36, MapID: 1436},                                                                                                     // one install: not corroborated
		{InstallID: "a", InstanceMapID: 33, MapID: 1421}, {InstallID: "b", InstanceMapID: 33, MapID: 1421}, {InstallID: "c", InstanceMapID: 33, MapID: 1411}, // a dissenter drops it
	}}
	got := BuildEntranceZones(c)
	if want := map[int32]EntranceZone{389: {UIMapID: 1454, Reporters: 2}}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %v want %v", got, want)
	}
}

func TestFetchCorpusReadsInstanceKinds(t *testing.T) {
	Delay = time.Millisecond
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == SettledPath {
			json.NewEncoder(w).Encode(map[string]any{"quest": []int64{}, "vendor": []int64{}})
			return
		}
		var recs []map[string]any
		switch r.URL.Query().Get("kinds") {
		case KindEncounterRec:
			recs = []map[string]any{{"kind": "encounter", "installId": "a", "encounterId": 5, "instanceMapId": 36, "success": true, "name": "x"}}
		case KindEntranceRec:
			recs = []map[string]any{{"kind": "instance_entrance", "installId": "a", "instanceMapId": 36, "mapId": 1436, "x": 1.5}}
		}
		json.NewEncoder(w).Encode(map[string]any{"dataAsOf": "d", "records": recs, "next": nil})
	}))
	defer srv.Close()
	c, err := FetchCorpus(srv.URL, t.TempDir(), "now", "b1")
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Encounters) != 1 || c.Encounters[0].EncounterID != 5 || len(c.Entrances) != 1 || c.Entrances[0].MapID != 1436 {
		t.Errorf("corpus: %+v", c)
	}
}
