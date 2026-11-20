package tables

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/WoW-Companion/data/internal/fetch"
	"github.com/WoW-Companion/data/internal/manifest"
	"github.com/WoW-Companion/data/internal/parquetio"
)

// SourceFrom turns a cache provenance record into a manifest source.
func SourceFrom(p fetch.Provenance) manifest.Source {
	return manifest.Source{
		Name:      fmt.Sprintf("%s (%s: %s)", p.Source, p.Product, join(p.Tables)),
		Version:   p.Build,
		Retrieved: p.Retrieved,
		License:   p.License,
	}
}

func join(s []string) string {
	out := ""
	for i, v := range s {
		if i > 0 {
			out += ", "
		}
		out += v
	}
	return out
}

func readCSVFile(dir, table string) ([]Row, error) {
	f, err := os.Open(filepath.Join(dir, table+".csv"))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return ReadCSV(f)
}

// Cached holds the parsed client tables of one cached build.
type Cached struct {
	Prov   fetch.Provenance
	Items  []Item
	Quests []Quest
}

// LoadCache parses the cache directory of one build into table rows.
func LoadCache(dir string) (Cached, error) {
	var c Cached
	b, err := os.ReadFile(filepath.Join(dir, fetch.ProvenanceFile))
	if err != nil {
		return c, err
	}
	if err := json.Unmarshal(b, &c.Prov); err != nil {
		return c, err
	}
	rows := map[string][]Row{}
	for _, t := range fetch.Tables {
		if rows[t], err = readCSVFile(dir, t); err != nil {
			return c, err
		}
	}
	c.Items = BuildItems(rows["ItemSparse"], rows["Item"])
	c.Quests = BuildQuests(rows["QuestV2"], rows["QuestLineXQuest"], rows["QuestPOIBlob"])
	return c, nil
}

// Layout is the adopted Parquet layout for the dataset's tables.
func Layout(bloom []string, src manifest.Source) parquetio.Options {
	return parquetio.Options{
		RowGroup: parquetio.DefaultRowGroup,
		Bloom:    bloom,
		Sort:     []string{"id"},
		Sources:  []manifest.Source{src},
	}
}

// BuildDataset writes items.parquet, quests.parquet and manifest.json into outDir.
func BuildDataset(cacheBuildDir, outDir, created string) (manifest.Manifest, error) {
	c, err := LoadCache(cacheBuildDir)
	if err != nil {
		return manifest.Manifest{}, err
	}
	src := SourceFrom(c.Prov)
	opts := Layout(nil, src) // no Bloom filters: see docs/spikes/parquet-layout.md
	m := manifest.Manifest{DatasetVersion: c.Prov.Build, GameBuild: c.Prov.Build, Created: created}
	rows, sum, err := parquetio.WriteFile(filepath.Join(outDir, "items.parquet"), c.Items, opts)
	if err != nil {
		return m, err
	}
	m.Tables = append(m.Tables, manifest.Table{Path: "items.parquet", Rows: rows, SHA256: sum, SortKeys: opts.Sort, Sources: opts.Sources})
	rows, sum, err = parquetio.WriteFile(filepath.Join(outDir, "quests.parquet"), c.Quests, opts)
	if err != nil {
		return m, err
	}
	m.Tables = append(m.Tables, manifest.Table{Path: "quests.parquet", Rows: rows, SHA256: sum, SortKeys: opts.Sort, Sources: opts.Sources})
	return m, manifest.Write(m, filepath.Join(outDir, "manifest.json"))
}
