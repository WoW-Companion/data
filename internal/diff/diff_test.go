package diff

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/WoW-Companion/data/internal/manifest"
	"github.com/WoW-Companion/data/internal/schema"
)

func ds(version, build string, rows map[string]int64, stats ...schema.TableCoverage) Dataset {
	d := Dataset{Manifest: manifest.Manifest{DatasetVersion: version, GameBuild: build}}
	for _, n := range []string{"items", "quests", "npcs"} {
		if r, ok := rows[n]; ok {
			d.Manifest.Tables = append(d.Manifest.Tables, manifest.Table{Path: n + ".parquet", Rows: r})
		}
	}
	if stats != nil {
		d.Stats = map[string]schema.TableCoverage{}
		for _, s := range stats {
			d.Stats[s.Name] = s
		}
	}
	return d
}

func tc(name string, rows int64, cols ...schema.ColumnCoverage) schema.TableCoverage {
	return schema.TableCoverage{Name: name, Rows: rows, Columns: cols}
}

func col(name, typ string, null, zero int64) schema.ColumnCoverage {
	return schema.ColumnCoverage{Name: name, Type: typ, Null: null, Zero: zero}
}

const header = "## Changes since the previous dataset\n\n| | Old | New |\n| --- | --- | --- |\n"

func TestMarkdown(t *testing.T) {
	tests := []struct {
		name     string
		old, cur Dataset
		want     string
	}{
		{
			"every change type",
			ds("1.0.0", "1.1", map[string]int64{"items": 100, "quests": 10},
				tc("items", 100, col("id", "INT32", 0, 0), col("name", "STRING", 0, 0), col("old_col", "INT32", 0, 0), col("slot", "INT32", 50, 0)),
				tc("quests", 10, col("id", "INT32", 0, 0))),
			ds("1.1.0", "1.2", map[string]int64{"items": 110, "npcs": 5},
				tc("items", 110, col("id", "INT64", 0, 0), col("name", "STRING", 0, 0), col("new_col", "INT32", 0, 0), col("slot", "INT32", 11, 0)),
				tc("npcs", 5, col("id", "INT32", 0, 0))),
			header + "| dataset_version | 1.0.0 | 1.1.0 |\n| game build | 1.1 | 1.2 |\n" +
				"\n### Tables\n\n- Added: `npcs`\n- Removed: `quests`\n" +
				"\n### Row counts\n\n| Table | Old | New | Change |\n| --- | ---: | ---: | ---: |\n| items | 100 | 110 | +10.0% |\n| npcs | - | 5 | n/a |\n| quests | 10 | - | n/a |\n" +
				"\n### Columns\n\n- `npcs.id` added (INT32)\n- `quests.id` removed (INT32)\n- `items.id` type changed: INT32 -> INT64\n- `items.new_col` added (INT32)\n- `items.old_col` removed (INT32)\n" +
				"\n### Coverage moves over 5 points\n\n- `items.slot` null share 50.0% -> 10.0% (-40.0 points)\n",
		},
		{
			"no changes, coverage under threshold",
			ds("1.0.0", "1.1", map[string]int64{"items": 100}, tc("items", 100, col("id", "INT32", 10, 0))),
			ds("1.0.1", "1.1", map[string]int64{"items": 100}, tc("items", 100, col("id", "INT32", 15, 0))),
			header + "| dataset_version | 1.0.0 | 1.0.1 |\n| game build | 1.1 | 1.1 |\n" +
				"\n### Tables\n\n- Added: none\n- Removed: none\n" +
				"\n### Row counts\n\n| Table | Old | New | Change |\n| --- | ---: | ---: | ---: |\n| items | 100 | 100 | 0.0% |\n" +
				"\n### Columns\n\nNo columns added, removed or type-changed.\n" +
				"\n### Coverage moves over 5 points\n\nNone.\n",
		},
		{
			"old side has no stats",
			ds("1.0.0", "1.1", map[string]int64{"items": 0}),
			ds("1.0.1", "1.1", map[string]int64{"items": 4}, tc("items", 4, col("id", "INT32", 0, 0))),
			header + "| dataset_version | 1.0.0 | 1.0.1 |\n| game build | 1.1 | 1.1 |\n" +
				"\n### Tables\n\n- Added: none\n- Removed: none\n" +
				"\n### Row counts\n\n| Table | Old | New | Change |\n| --- | ---: | ---: | ---: |\n| items | 0 | 4 | n/a |\n" +
				"\n### Columns\n\nColumn and coverage changes are not available: one side has no stats.\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Markdown(tt.old, tt.cur); got != tt.want {
				t.Errorf("got:\n%s\nwant:\n%s", got, tt.want)
			}
		})
	}
}

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	m := `{"dataset_version":"1.0.0","game_build":"1.1","created":"2026-01-01T00:00:00Z","tables":[{"path":"items.parquet","rows":3}]}`
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(m), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "stats.json"), []byte(`[{"name":"items","rows":3,"columns":[{"name":"id","type":"INT32","null":0,"zero":1}]},{"name":"npcs","rows":0,"missing":true,"columns":null}]`), 0o644); err != nil {
		t.Fatal(err)
	}
	d, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if d.Manifest.GameBuild != "1.1" || len(d.Stats) != 1 || d.Stats["items"].Columns[0].Zero != 1 {
		t.Errorf("unexpected dataset: %+v", d)
	}
	if _, err := Load(t.TempDir()); err == nil {
		t.Error("a directory without manifest.json must fail")
	}
}
