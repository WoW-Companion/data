package tables

import (
	"net/http"
	"net/http/httptest"

	"github.com/WoW-Companion/data/internal/everloot"
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
	// the random suffix tables live in their own build's cache, beside this one
	sfx := filepath.Join(dir, "cache", fetch.RandomSuffixBuild)
	os.MkdirAll(sfx, 0o755)
	for tb, body := range map[string]string{
		"ItemRandomProperties": "ID,Name_lang,Enchantment_0,Enchantment_4\n",
		"ItemRandomSuffix":     "ID,Name_lang,Enchantment_0,Enchantment_4,AllocationPct_0\n6,of the Eagle,2804,0,6666\n",
		"SpellItemEnchantment": "ID,Name_lang,Effect_0,Effect_2,EffectArg_0,EffectArg_2,EffectPointsMin_0,EffectPointsMin_2\n2804,+$i Intellect,5,0,5,0,0,0\n",
	} {
		os.WriteFile(filepath.Join(sfx, tb+".csv"), []byte(body), 0o644)
	}
	os.WriteFile(filepath.Join(sfx, fetch.ProvenanceFile), []byte(`{"source":"wago.tools","product":"wowf","build":"`+fetch.RandomSuffixBuild+`","retrieved":"2026-11-20T00:00:00Z","license":"x","tables":["ItemRandomSuffix"]}`), 0o644)
	os.WriteFile(filepath.Join(cache, fetch.ProvenanceFile), []byte(`{"source":"wago.tools","product":"wowf","build":"b1","retrieved":"2026-11-20T00:00:00Z","license":"x","tables":["Creature"]}`), 0o644)
	ev := filepath.Join(dir, "drops.json")
	os.WriteFile(ev, []byte(`{"retrieved":"2026-11-20T00:00:00Z","dataAsOf":"2026-11-19","rates":[{"itemId":1,"sourceNpcId":5,"kills":4,"drops":1,"rate":0.25,"reporters":3,"public":true}]}`), 0o644)
	out := filepath.Join(dir, "out")
	inst := filepath.Join(dir, "instances.yaml")
	os.WriteFile(inst, []byte("status: \"advisor default, not authoritative\"\ninstances:\n"), 0o644)
	prof := filepath.Join(dir, "professions.yaml")
	os.WriteFile(prof, []byte("status: \"advisor default, not authoritative\"\n"), 0o644)
	m, err := BuildDataset(cache, ev, filepath.Join(dir, "no-builds"), inst, prof, out, "2026-11-20T00:00:00Z", "")
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	for _, tb := range m.Tables {
		paths = append(paths, tb.Path)
	}
	if got := strings.Join(paths, ","); got != "items.parquet,quests.parquet,npcs.parquet,talent_trees.parquet,talent_nodes.parquet,talent_builds.parquet,talent_priorities.parquet,stat_types.parquet,recipes.parquet,recipe_reagents.parquet,class_spells.parquet,weapon_skills.parquet,weapon_masters.parquet,instances.parquet,instance_bosses.parquet,instance_quests.parquet,loot_sources.parquet,quests_observed.parquet,vendor_sources.parquet,item_sets.parquet,random_suffixes.parquet,quest_lines.parquet,recipe_items.parquet,trainers.parquet,trainer_spells.parquet,gathering_nodes.parquet,gathering_yields.parquet,fishing_catches.parquet,disenchant_yields.parquet,profession_notes.parquet,profession_routes.parquet,profession_tiers.parquet,names.parquet" {
		t.Fatalf("tables: %s", got)
	}
	if m.Tables[2].Rows != 1 || m.Tables[16].Rows != 1 || m.Tables[7].Rows != int64(len(StatTypeLabels)) {
		t.Errorf("rows: %+v", m.Tables)
	}
	if err := manifest.VerifyFile(filepath.Join(out, "manifest.json")); err != nil {
		t.Fatal(err)
	}
	sb, _ := os.ReadFile(filepath.Join(out, "random_suffixes.parquet"))
	if ssrc, err := parquetio.ReadSources(sb); err != nil || len(ssrc) != 1 || ssrc[0].Version != fetch.RandomSuffixBuild {
		t.Errorf("random_suffixes source: %+v %v", ssrc, err)
	}
	b, _ := os.ReadFile(filepath.Join(out, "loot_sources.parquet"))
	srcs, err := parquetio.ReadSources(b)
	if err != nil || len(srcs) != 2 || !strings.Contains(srcs[0].License, "Data from Everloot (https://everloot.app), CC BY-SA 4.0.") || srcs[0].Version != "2026-11-19" {
		t.Errorf("attribution missing from Parquet metadata: %+v %v", srcs, err)
	}
}

// A rate-limited Everloot (429 on every route) must not fail the fetch or the build: the
// sources come back unavailable and every table built from them is empty.
func TestBuildSurvivesEverlootRateLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "2973")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()
	old := everloot.Delay
	everloot.Delay = 0
	defer func() { everloot.Delay = old }()

	dir := t.TempDir()
	evDir := filepath.Join(dir, "cache", "everloot")
	_, _, _, status := everloot.FetchAll(srv.URL, "", evDir, "2026-11-20T00:00:00Z", "b1")
	if len(status) != 3 {
		t.Fatalf("status: %+v", status)
	}
	for _, s := range status {
		if s.Err == nil || !strings.Contains(s.Line(), "UNAVAILABLE") {
			t.Errorf("want unavailable, got %q", s.Line())
		}
	}
	if ents, _ := os.ReadDir(evDir); len(ents) != 0 {
		t.Errorf("a failed fetch cached %d files", len(ents))
	}

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
	out := filepath.Join(dir, "out")
	if _, err := BuildDataset(cache, filepath.Join(evDir, everloot.CacheFile), filepath.Join(dir, "no-builds"), inst, prof, out, "2026-11-20T00:00:00Z", ""); err != nil {
		t.Fatal(err)
	}
	if err := manifest.VerifyFile(filepath.Join(out, "manifest.json")); err != nil {
		t.Fatal(err)
	}
}
