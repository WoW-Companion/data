package everloot

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

const stamp = "2026-10-19T06-40-12-328Z"

// exportServer serves latest.json, an index and files; hits counts requests per path.
type exportServer struct {
	mu      sync.Mutex
	hits    map[string]int
	files   map[string]string // name -> body
	lie     string            // a file whose index checksum is wrong
	drops   int               // rows in drops.ndjson
	noIndex bool
}

func (e *exportServer) index() string {
	var fs []string
	for _, f := range []struct{ name, ds, field string }{{"drops.ndjson", "drops", "rates"}, {"edges.ndjson", "edges", "edges"}, {"corpus.ndjson", "corpus", "records"}} {
		body := e.files[f.name]
		sum := sha256.Sum256([]byte(body))
		h := hex.EncodeToString(sum[:])
		if f.name == e.lie {
			h = strings.Repeat("0", 64)
		}
		rows := strings.Count(body, "\n")
		fs = append(fs, fmt.Sprintf(`{"name":%q,"dataset":%q,"format":"ndjson","rowsOf":"x","field":%q,"rows":%d,"bytes":%d,"sha256":%q,"data":"observed","attribution":null,"gameData":null}`, f.name, f.ds, f.field, rows, len(body), h))
	}
	return `{"protocol":"1.1.0","generatedAt":"2026-10-19T06:40:12.328Z","dataAsOf":"2026-10-19T06:40:12.338Z","build":"1.0","site":"https://everloot.app","licence":"CC BY-SA 4.0","licenceUrl":"u","credit":"c","files":[` + strings.Join(fs, ",") + `]}`
}

func (e *exportServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	e.mu.Lock()
	e.hits[r.URL.Path]++
	e.mu.Unlock()
	switch {
	case r.URL.Path == "/latest.json":
		fmt.Fprintf(w, `{"stamp":%q,"index":%q}`, stamp, stamp+"/index.json")
	case r.URL.Path == "/"+stamp+"/index.json" && !e.noIndex:
		w.Write([]byte(e.index()))
	case strings.HasPrefix(r.URL.Path, "/"+stamp+"/"):
		if b, ok := e.files[strings.TrimPrefix(r.URL.Path, "/"+stamp+"/")]; ok {
			w.Write([]byte(b))
			return
		}
		http.NotFound(w, r)
	default:
		http.NotFound(w, r)
	}
}

func newExport() *exportServer {
	return &exportServer{hits: map[string]int{}, files: map[string]string{
		"drops.ndjson": `{"sourceNpcId":7,"itemId":9,"kills":10,"drops":3,"rate":0.3,"reporters":4,"public":true,"sourceName":"Bob"}` + "\n",
		"edges.ndjson": `{"fromKind":"item","fromId":9,"rel":"reward_from","toKind":"quest","toId":5,"installs":null}` + "\n" +
			`{"fromKind":"npc","fromId":7,"rel":"located_in","toKind":"zone","toId":1,"installs":2}` + "\n" +
			`{"fromKind":"item","fromId":1,"rel":"created_by","toKind":"spell","toId":2,"installs":null}` + "\n",
		"corpus.ndjson": `{"kind":"quest","installId":"a","questId":5,"title":"Q"}` + "\n" + `{"kind":"vendor","installId":"a","vendorNpcId":3,"items":[]}` + "\n",
	}}
}

func TestExportIndexFilesAndImmutableCache(t *testing.T) {
	es := newExport()
	srv := httptest.NewServer(es)
	defer srv.Close()
	dir := t.TempDir()
	ex, err := FetchExport(srv.URL, dir)
	if err != nil {
		t.Fatal(err)
	}
	if ex.Origin() != "export "+stamp+"/index.json" || ex.Index.DataAsOf != "2026-10-19T06:40:12.338Z" {
		t.Fatalf("export: %+v", ex)
	}
	rates, ok, err := ex.Rates()
	if err != nil || !ok || len(rates) != 1 || rates[0].SourceName != "Bob" {
		t.Fatalf("rates %+v %v %v", rates, ok, err)
	}
	if e, _, err := ex.Edges(RelReward); err != nil || len(e) != 1 {
		t.Fatalf("edges %+v %v", e, err)
	}
	before := es.hits["/"+stamp+"/edges.ndjson"]
	if _, err := FetchExport(srv.URL, dir); err != nil {
		t.Fatal(err)
	}
	if es.hits["/"+stamp+"/edges.ndjson"] != before || before != 1 || es.hits["/"+stamp+"/index.json"] != 1 {
		t.Fatalf("a cached immutable folder was fetched again: %v", es.hits)
	}
}

func TestExportChecksumMismatchRejected(t *testing.T) {
	es := newExport()
	es.lie = "edges.ndjson"
	srv := httptest.NewServer(es)
	defer srv.Close()
	if _, err := FetchExport(srv.URL, t.TempDir()); err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("err = %v", err)
	}
}

func TestExportBadIndexRejected(t *testing.T) {
	for _, body := range []string{`{"files":[]}`, `not json`} {
		if err := ValidateIndex([]byte(body)); err == nil {
			t.Errorf("accepted %q", body)
		}
	}
}

func TestExportReplacesAPIAndEmptyFilesFallBack(t *testing.T) {
	Delay = time.Millisecond
	es := newExport()
	srv := httptest.NewServer(es)
	defer srv.Close()
	api := &fake{}
	apiSrv := httptest.NewServer(api)
	defer apiSrv.Close()

	// All three files have rows: the API is asked for nothing but the settled lists.
	dir := t.TempDir()
	ev, cor, _, st := FetchAll(apiSrv.URL, srv.URL, dir, "now", "1.0")
	for _, s := range st[:3] { // the disenchant rollup is not in the fake
		if s.Err != nil {
			t.Fatalf("%s", s.Line())
		}
	}
	if !strings.HasPrefix(ev.Origin, "export ") || len(ev.Rates) != 1 || len(ev.QuestRewards) != 1 || len(ev.NpcZones) != 1 || ev.DataAsOf != "2026-10-19T06:40:12.338Z" {
		t.Fatalf("snapshot: %+v", ev)
	}
	if len(cor.Quests) != 1 || len(cor.Vendors) != 1 || !strings.HasPrefix(cor.Origin, "export ") {
		t.Fatalf("corpus: %+v", cor)
	}
	for _, p := range api.paths {
		if strings.HasPrefix(p, DropsPath) || strings.HasPrefix(p, EdgesPath) || strings.HasPrefix(p, CorpusPath) {
			t.Fatalf("API asked for exported data: %s", p)
		}
	}

	// An export with no drops rows (storage was down when it was made) pages the API for them.
	es2 := newExport()
	es2.files["drops.ndjson"] = ""
	srv2 := httptest.NewServer(es2)
	defer srv2.Close()
	ev, _, _, _ = FetchAll(apiSrv.URL, srv2.URL, t.TempDir(), "now", "1.0")
	if ev.Origin != "export "+stamp+"/index.json + api" {
		t.Fatalf("origin %q", ev.Origin)
	}
}

func TestRelationMissingFromExportComesFromAPI(t *testing.T) {
	Delay = time.Millisecond
	es := newExport()
	es.files["edges.ndjson"] = `{"fromKind":"item","fromId":1,"rel":"created_by","toKind":"spell","toId":2,"installs":null}` + "\n"
	srv := httptest.NewServer(es)
	defer srv.Close()
	api := &fake{}
	apiSrv := httptest.NewServer(api)
	defer apiSrv.Close()
	ev, _, _, _ := FetchAll(apiSrv.URL, srv.URL, t.TempDir(), "now", "1.0")
	asked := 0
	for _, p := range api.paths {
		if strings.HasPrefix(p, EdgesPath) {
			asked++
		}
	}
	if asked != 2 || ev.Origin != "export "+stamp+"/index.json + api" {
		t.Fatalf("asked %d, origin %q", asked, ev.Origin)
	}
}

func TestFallbackToAPIWhenExportUnreachable(t *testing.T) {
	Delay = time.Millisecond
	es := newExport()
	es.noIndex = true
	srv := httptest.NewServer(es)
	defer srv.Close()
	api := &fake{}
	apiSrv := httptest.NewServer(api)
	defer apiSrv.Close()
	ev, _, _, st := FetchAll(apiSrv.URL, srv.URL, t.TempDir(), "now", "1.0")
	if ev.Origin != OriginAPI {
		t.Fatalf("origin %q", ev.Origin)
	}
	if st[0].Err == nil || !strings.Contains(st[0].Line(), "falling back to the API") {
		t.Fatalf("status %q", st[0].Line())
	}
	// Both unreachable: the sources are reported unavailable, left for carry-forward.
	api.fail = true
	_, _, _, st = FetchAll(apiSrv.URL, srv.URL, t.TempDir(), "now", "1.0")
	if st[1].Err == nil {
		t.Fatalf("status %+v", st)
	}
}

func TestSourceVersion(t *testing.T) {
	for _, c := range []struct{ o, d, want string }{
		{"", "2026-11-19", "2026-11-19"},
		{"api", "2026-11-19", "api dataAsOf 2026-11-19"},
		{"export a/index.json", "x", "export a/index.json dataAsOf x"},
		{"api", "unavailable", "unavailable"},
	} {
		if got := SourceVersion(c.o, c.d); got != c.want {
			t.Errorf("%q", got)
		}
	}
}
