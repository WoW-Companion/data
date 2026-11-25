package tables

import (
	"archive/tar"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/WoW-Companion/data/internal/carry"
	"github.com/WoW-Companion/data/internal/everloot"
	"github.com/WoW-Companion/data/internal/fetch"
	"github.com/WoW-Companion/data/internal/manifest"
	"github.com/WoW-Companion/data/internal/parquetio"
	"github.com/klauspost/compress/zstd"
)

type fakeRel struct {
	tag, rev, published string
	version             string // the Everloot source version in the manifest
	vendors             []everloot.VendorSource
}

// fakeGitHub serves the release list, each manifest and each archive of rels.
func fakeGitHub(t *testing.T, rels []fakeRel) *httptest.Server {
	t.Helper()
	files := map[string][]byte{}
	var list []map[string]any
	for _, r := range rels {
		list = append(list, map[string]any{"tag_name": r.tag, "published_at": r.published})
		build := strings.TrimPrefix(r.tag, "dataset-")
		src := manifest.Source{Name: everloot.CorpusName, Version: r.version, Retrieved: r.published, License: "x"}
		pq, err := parquetio.Encode(r.vendors, Layout(nil, []string{"npc_id", "item_id"}, src))
		if err != nil {
			t.Fatal(err)
		}
		tb := manifest.Table{Path: "vendor_sources.parquet", Rows: int64(len(r.vendors)), SHA256: sum(pq), SortKeys: []string{"npc_id"}, Sources: []manifest.Source{src}}
		mb, err := manifest.Encode(manifest.Manifest{DatasetVersion: build, GameBuild: build, Created: r.published, Revision: r.rev, Tables: []manifest.Table{tb}})
		if err != nil {
			t.Fatal(err)
		}
		var tarBuf bytes.Buffer
		tw := tar.NewWriter(&tarBuf)
		tw.WriteHeader(&tar.Header{Name: build + "/vendor_sources.parquet", Mode: 0o644, Size: int64(len(pq)), Typeflag: tar.TypeReg})
		tw.Write(pq)
		tw.Close()
		var zbuf bytes.Buffer
		zw, _ := zstd.NewWriter(&zbuf)
		zw.Write(tarBuf.Bytes())
		zw.Close()
		base := "/o/r/releases/download/" + r.tag + "/"
		files[base+"manifest-"+build+".json"] = mb
		files[base+"wowc-dataset-"+build+".tar.zst"] = zbuf.Bytes()
	}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path == "/repos/o/r/releases" {
			json.NewEncoder(w).Encode(list)
			return
		}
		if b, ok := files[req.URL.Path]; ok {
			w.Write(b)
			return
		}
		http.NotFound(w, req)
	}))
}

func sum(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func vendorRows(n int) []everloot.VendorSource {
	out := make([]everloot.VendorSource, n)
	for i := range out {
		out[i] = everloot.VendorSource{NpcID: int32(i + 1), ItemID: 7, PriceCopper: 100, Reporters: 3}
	}
	return out
}

// buildWithoutEverloot builds against a 429 Everloot, carrying from the fake releases.
func buildWithoutEverloot(t *testing.T, rels []fakeRel) manifest.Manifest {
	t.Helper()
	gh := fakeGitHub(t, rels)
	defer gh.Close()
	rate := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "2973")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer rate.Close()
	old := everloot.Delay
	everloot.Delay = 0
	defer func() { everloot.Delay = old }()

	dir := t.TempDir()
	evDir := filepath.Join(dir, "cache", "everloot")
	everloot.FetchAll(rate.URL, evDir, "2026-11-20T00:00:00Z", "b1")
	cache := filepath.Join(dir, "cache", "b1")
	os.MkdirAll(cache, 0o755)
	for _, tb := range fetch.Tables {
		os.WriteFile(filepath.Join(cache, tb+".csv"), []byte(strings.Join(Required[tb], ",")+"\n"), 0o644)
	}
	os.WriteFile(filepath.Join(cache, fetch.ProvenanceFile), []byte(`{"source":"wago.tools","product":"wowf","build":"b1","retrieved":"2026-11-20T00:00:00Z","license":"x","tables":["Creature"]}`), 0o644)
	inst := filepath.Join(dir, "instances.yaml")
	os.WriteFile(inst, []byte("status: \"advisor default, not authoritative\"\ninstances:\n"), 0o644)
	prof := filepath.Join(dir, "professions.yaml")
	os.WriteFile(prof, []byte("status: \"advisor default, not authoritative\"\n"), 0o644)
	f := &carry.Finder{API: gh.URL, Download: gh.URL, Repo: "o/r", Build: "b1", HTTP: &http.Client{Timeout: 10 * time.Second}}
	m, err := BuildDatasetFrom(cache, filepath.Join(evDir, everloot.CacheFile), filepath.Join(dir, "none"), inst, prof, filepath.Join(dir, "out"), "2026-11-25T00:00:00Z", "", f)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func vendorTable(t *testing.T, m manifest.Manifest) manifest.Table {
	t.Helper()
	for _, tb := range m.Tables {
		if tb.Path == "vendor_sources.parquet" {
			return tb
		}
	}
	t.Fatal("no vendor_sources table")
	return manifest.Table{}
}

// With Everloot answering 429, the table comes from the newest release that fetched it,
// skipping a newer release that was itself built empty.
func TestCarryForwardWhenEverlootLimited(t *testing.T) {
	m := buildWithoutEverloot(t, []fakeRel{
		{tag: "dataset-b1", rev: "e0e0e0e0e0e0", published: "2026-11-25T01:00:00Z", version: carry.Unavailable, vendors: nil},
		{tag: "dataset-b0", rev: "90090090090a", published: "2026-11-20T01:00:00Z", version: "2026-11-19", vendors: vendorRows(3)},
	})
	v := vendorTable(t, m)
	if v.Rows != 3 {
		t.Fatalf("vendor_sources rows = %d, want 3 carried forward", v.Rows)
	}
	s := v.Sources[0]
	if s.Version != carry.Carried || !strings.Contains(s.Name, "90090090090a") || s.Retrieved != "2026-11-20T01:00:00Z" {
		t.Errorf("source = %+v", s)
	}
}

// A release carried forward from a fetch is itself a good source; one built empty is not,
// and with nothing earlier the table builds empty.
func TestCarryForwardNeverFromEmptyRelease(t *testing.T) {
	m := buildWithoutEverloot(t, []fakeRel{
		{tag: "dataset-b1", rev: "e0e0e0e0e0e0", published: "2026-11-25T01:00:00Z", version: carry.Unavailable, vendors: nil},
	})
	if v := vendorTable(t, m); v.Rows != 0 {
		t.Fatalf("rows = %d, want 0 when no release has the source", v.Rows)
	}
	m = buildWithoutEverloot(t, []fakeRel{
		{tag: "dataset-b1", rev: "ca0ca0ca0ca0", published: "2026-11-25T01:00:00Z", version: carry.Carried, vendors: vendorRows(2)},
	})
	if v := vendorTable(t, m); v.Rows != 2 || v.Sources[0].Version != carry.Carried {
		t.Fatalf("carried-forward release with rows should carry again: %+v", v)
	}
}
