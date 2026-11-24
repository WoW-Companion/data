package schema

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/WoW-Companion/data/internal/fetch"
	"github.com/WoW-Companion/data/internal/tables"
)

func TestColumns(t *testing.T) {
	type row struct {
		A int32      `parquet:"a"`
		B *string    `parquet:"b,optional"`
		C []int16    `parquet:"c,list"`
		D []struct { // not a registry type; only the tag reading is checked
			X int16  `parquet:"x"`
			Y *int32 `parquet:"y,optional"`
		} `parquet:"d,list"`
	}
	want := []Column{
		{"a", "int32", false}, {"b", "string", true}, {"c", "list<int16>", false},
		{"d", "list<struct{x int16, y int32?}>", false},
	}
	got := Columns(row{})
	for i, w := range want {
		if got[i] != w {
			t.Errorf("column %d: got %+v, want %+v", i, got[i], w)
		}
	}
}

// Every struct column is documented, in order, and nothing documented is stale.
func TestRegistryDocumentsEveryColumn(t *testing.T) {
	for _, tb := range Tables {
		cols := Columns(tb.Row)
		if len(cols) != len(tb.Cols) {
			t.Errorf("%s: %d columns, %d documented", tb.Name, len(cols), len(tb.Cols))
			continue
		}
		for i, c := range cols {
			if tb.Cols[i].Name != c.Name || tb.Cols[i].Meaning == "" {
				t.Errorf("%s column %d: %q documented as %q", tb.Name, i, c.Name, tb.Cols[i].Name)
			}
		}
	}
}

func TestSchemaFileIsCurrent(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "docs", "schema.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != Markdown() {
		t.Fatal("docs/schema.md is stale: run `go run ./cmd/wowc-data schema`")
	}
}

// One fixture dataset through BuildDataset: the registry lists exactly the tables written,
// and coverage counts nulls, zeros and empty lists in them.
func TestCoverageOnFixtureDataset(t *testing.T) {
	dir := t.TempDir()
	cache := filepath.Join(dir, "cache", "b1")
	os.MkdirAll(cache, 0o755)
	for _, tb := range fetch.Tables {
		body := strings.Join(tables.Required[tb], ",") + "\n"
		switch tb {
		case "Creature":
			body += "5,Wolf,1,0,0\n"
		case "ItemSparse":
			// ID, Display_lang, ItemLevel, RequiredLevel, OverallQualityID, InventoryType, AllowableClass, Bonding, stat, pct, delay
			body += "1,Sword,5,0,2,13,-1,0,3,10000,0\n2,Rock,1,0,1,0,-1,0,-1,0,0\n"
		}
		os.WriteFile(filepath.Join(cache, tb+".csv"), []byte(body), 0o644)
	}
	os.WriteFile(filepath.Join(cache, fetch.ProvenanceFile), []byte(`{"source":"wago.tools","product":"wowf","build":"b1","retrieved":"2026-11-20T00:00:00Z","license":"x","tables":["Creature"]}`), 0o644)
	ev := filepath.Join(dir, "drops.json")
	os.WriteFile(ev, []byte(`{"retrieved":"2026-11-20T00:00:00Z","dataAsOf":"2026-11-19","rates":[]}`), 0o644)
	out := filepath.Join(dir, "out")
	inst := filepath.Join(dir, "instances.yaml")
	os.WriteFile(inst, []byte("status: \"advisor default, not authoritative\"\ninstances:\n"), 0o644)
	prof := filepath.Join(dir, "professions.yaml")
	os.WriteFile(prof, []byte("status: \"advisor default, not authoritative\"\n"), 0o644)
	m, err := tables.BuildDataset(cache, ev, filepath.Join(dir, "none"), inst, prof, out, "2026-11-20T00:00:00Z", "")
	if err != nil {
		t.Fatal(err)
	}
	for i, tb := range m.Tables {
		if Tables[i].File() != tb.Path {
			t.Fatalf("registry table %d is %s, dataset wrote %s", i, Tables[i].File(), tb.Path)
		}
	}
	if len(m.Tables) != len(Tables) {
		t.Fatalf("registry has %d tables, dataset %d", len(Tables), len(m.Tables))
	}
	tcs, err := Coverage(out)
	if err != nil {
		t.Fatal(err)
	}
	md := CoverageMarkdown("b1", tcs)
	for _, want := range []string{
		"| items | 2 |",
		"| `armor` | 100.0% | 0.0% |",
		"| `stat_types` | 50.0% | 0.0% |", // item 2 lists no stat: an empty list
		"| `stats.type` | 50.0% | 0.0% |",
		"| `stats.value` | 100.0% | 0.0% |",
		"| `bonding` | 0.0% | 100.0% |",
		"| `slot` | 0.0% | 50.0% |",
		"| npcs | 1 |",
		"| loot_sources | 0 |",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("coverage lacks %q", want)
		}
	}
	os.Remove(filepath.Join(out, "recipes.parquet"))
	tcs, _ = Coverage(out)
	if md := CoverageMarkdown("b1", tcs); !strings.Contains(md, "| recipes | missing |") {
		t.Error("a missing file is not reported")
	}
}
