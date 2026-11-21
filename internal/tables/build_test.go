package tables

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/WoW-Companion/data/internal/fetch"
	"github.com/WoW-Companion/data/internal/manifest"
	"github.com/WoW-Companion/data/internal/parquetio"
)

// One end-to-end pass over fixture files: every table is written, listed, and verifies.
func TestBuildDatasetSmoke(t *testing.T) {
	dir := t.TempDir()
	cache := filepath.Join(dir, "cache", "b1")
	os.MkdirAll(cache, 0o755)
	for _, tb := range fetch.Tables {
		body := strings.Join(Required[tb], ",") + "\n"
		if tb == "Creature" {
			body += "5,Wolf,1,0,0\n"
		}
		os.WriteFile(filepath.Join(cache, tb+".csv"), []byte(body), 0o644)
	}
	os.WriteFile(filepath.Join(cache, fetch.ProvenanceFile), []byte(`{"source":"wago.tools","product":"wowf","build":"b1","retrieved":"2026-11-20T00:00:00Z","license":"x","tables":["Creature"]}`), 0o644)
	ev := filepath.Join(dir, "drops.json")
	os.WriteFile(ev, []byte(`{"retrieved":"2026-11-20T00:00:00Z","dataAsOf":"2026-11-19","rates":[{"itemId":1,"sourceNpcId":5,"kills":4,"drops":1,"rate":0.25,"reporters":3,"public":true}]}`), 0o644)
	out := filepath.Join(dir, "out")
	m, err := BuildDataset(cache, ev, out, "2026-11-20T00:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	for _, tb := range m.Tables {
		paths = append(paths, tb.Path)
	}
	if got := strings.Join(paths, ","); got != "items.parquet,quests.parquet,npcs.parquet,talent_trees.parquet,talent_nodes.parquet,stat_types.parquet,recipes.parquet,recipe_reagents.parquet,instances.parquet,instance_bosses.parquet,loot_sources.parquet" {
		t.Fatalf("tables: %s", got)
	}
	if m.Tables[2].Rows != 1 || m.Tables[10].Rows != 1 || m.Tables[5].Rows != int64(len(StatTypeLabels)) {
		t.Errorf("rows: %+v", m.Tables)
	}
	if err := manifest.VerifyFile(filepath.Join(out, "manifest.json")); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(out, "loot_sources.parquet"))
	srcs, err := parquetio.ReadSources(b)
	if err != nil || len(srcs) != 1 || !strings.Contains(srcs[0].License, "Data from Everloot (https://everloot.app), CC BY-SA 4.0.") || srcs[0].Version != "2026-11-19" {
		t.Errorf("attribution missing from Parquet metadata: %+v %v", srcs, err)
	}
}
