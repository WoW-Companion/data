// Package fetch downloads client tables from wago.tools' CSV export into a local
// cache, once per build, one request at a time.
package fetch

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"time"
)

const (
	BaseURL        = "https://wago.tools"
	Product        = "wowf"
	Credit         = "Blizzard game client data, retrieved via wago.tools"
	LicenseBasis   = "Operator ruling 2026-11-20: client tables retrieved via wago.tools may be used; facts only, no long-form text"
	UserAgent      = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 Safari/605.1.15"
	ProvenanceFile = "provenance.json"
	DefaultBuild   = "1.60.1.71037"
)

// Delay is the pause between two downloads.
var Delay = 2 * time.Second

// Tables are the client tables the pipeline reads.
var Tables = []string{"ItemSparse", "Item", "QuestV2", "QuestLineXQuest", "QuestPOIBlob", "UiMap", "Creature", "CreatureDifficulty",
	"TraitNode", "TraitNodeGroup", "TraitNodeGroupXTraitNode", "TraitNodeXTraitNodeEntry", "TraitNodeEntry", "TraitDefinition", "TraitEdge", "TraitCond", "SkillLineXTraitTree", "SkillRaceClassInfo", "ChrClasses", "SpellName",
	"SkillLine", "SkillLineAbility", "SpellReagents", "SpellEffect",
	"Map", "DungeonEncounter", "MapDifficulty", "ContentTuning",
	"ItemSet", "QuestLine"}

// Provenance is the record kept beside the cached CSVs and stamped into every built table.
type Provenance struct {
	Source    string   `json:"source"`
	Credit    string   `json:"credit"`
	Product   string   `json:"product"`
	Build     string   `json:"build"`
	Retrieved string   `json:"retrieved"`
	License   string   `json:"license"`
	Tables    []string `json:"tables"`
}

// TableURL is the CSV export URL for a table at a build.
func TableURL(base, table, build string) string {
	return base + "/db2/" + table + "/csv?" + url.Values{"version": {build}}.Encode()
}

// MakeProvenance builds the provenance record with sorted table names.
func MakeProvenance(build, retrieved string, tables []string) Provenance {
	t := slices.Clone(tables)
	slices.Sort(t)
	return Provenance{
		Source: "wago.tools", Credit: Credit, Product: Product, Build: build,
		Retrieved: retrieved, License: LicenseBasis, Tables: t,
	}
}

// Build downloads tables for build into cacheDir/build, skipping files already cached,
// and returns the provenance written beside them.
func Build(baseURL, build, cacheDir string, tables []string) (Provenance, error) {
	raw := filepath.Join(cacheDir, build)
	if err := os.MkdirAll(raw, 0o755); err != nil {
		return Provenance{}, err
	}
	provPath := filepath.Join(raw, ProvenanceFile)
	var prov *Provenance
	if b, err := os.ReadFile(provPath); err == nil {
		prov = new(Provenance)
		if err := json.Unmarshal(b, prov); err != nil {
			return Provenance{}, fmt.Errorf("%s: %w", provPath, err)
		}
	}
	fetched := false
	client := &http.Client{Timeout: 120 * time.Second}
	for _, t := range tables {
		dest := filepath.Join(raw, t+".csv")
		if _, err := os.Stat(dest); err == nil {
			continue
		}
		if fetched {
			time.Sleep(Delay)
		}
		if err := download(client, TableURL(baseURL, t, build), dest); err != nil {
			return Provenance{}, fmt.Errorf("fetch %s: %w", t, err)
		}
		fetched = true
	}
	if prov == nil || fetched {
		retrieved := time.Now().UTC().Format("2006-01-02T15:04:05Z")
		if prov != nil {
			retrieved = prov.Retrieved
		}
		p := MakeProvenance(build, retrieved, tables)
		prov = &p
		b, _ := json.MarshalIndent(p, "", "  ")
		if err := os.WriteFile(provPath, append(b, '\n'), 0o644); err != nil {
			return Provenance{}, err
		}
	}
	return *prov, nil
}

func download(c *http.Client, u, dest string) error {
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", UserAgent)
	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	tmp := dest + ".part"
	if err := os.WriteFile(tmp, body, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, dest)
}
