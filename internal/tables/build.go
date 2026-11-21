package tables

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/WoW-Companion/data/internal/everloot"
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
	return ReadTable(table, f)
}

// Cached holds the parsed client tables of one cached build.
type Cached struct {
	Prov   fetch.Provenance
	Items  []Item
	Quests []Quest
	Npcs   []Npc

	TalentTrees []TalentTree
	TalentNodes []TalentNode

	Recipes        []Recipe
	RecipeReagents []RecipeReagent

	Instances      []Instance
	InstanceBosses []InstanceBoss
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
	c.Quests = BuildQuests(rows["QuestV2"], rows["QuestLineXQuest"], rows["QuestPOIBlob"], rows["UiMap"])
	c.Npcs = BuildNpcs(rows["Creature"], rows["CreatureDifficulty"])
	in := TalentInput{
		Groups: rows["TraitNodeGroup"], GroupNodes: rows["TraitNodeGroupXTraitNode"], Conds: rows["TraitCond"],
		SkillTrees: rows["SkillLineXTraitTree"], SkillClasses: rows["SkillRaceClassInfo"], Classes: rows["ChrClasses"],
		Nodes: rows["TraitNode"], NodeEntries: rows["TraitNodeXTraitNodeEntry"], Entries: rows["TraitNodeEntry"],
		Definitions: rows["TraitDefinition"], Edges: rows["TraitEdge"], SpellNames: rows["SpellName"],
	}
	c.TalentTrees = BuildTalentTrees(in)
	c.TalentNodes = BuildTalentNodes(in)
	rin := RecipeInput{
		SkillLines: rows["SkillLine"], Abilities: rows["SkillLineAbility"], Reagents: rows["SpellReagents"],
		Effects: rows["SpellEffect"], SpellNames: rows["SpellName"],
	}
	c.Recipes = BuildRecipes(rin)
	c.RecipeReagents = BuildRecipeReagents(c.Recipes, rin)
	iin := InstanceInput{
		Maps: rows["Map"], Encounters: rows["DungeonEncounter"], MapDifficulties: rows["MapDifficulty"],
		ContentTuning: rows["ContentTuning"], Creatures: rows["Creature"],
	}
	c.Instances = BuildInstances(iin)
	c.InstanceBosses = BuildInstanceBosses(c.Instances, iin)
	return c, nil
}

// Layout is the adopted Parquet layout for the dataset's tables.
func Layout(bloom, sort []string, srcs ...manifest.Source) parquetio.Options {
	return parquetio.Options{
		RowGroup: parquetio.DefaultRowGroup,
		Bloom:    bloom,
		Sort:     sort,
		Sources:  srcs,
	}
}

// EverlootSource turns a cached Everloot snapshot into a manifest source carrying its
// attribution.
func EverlootSource(s everloot.Snapshot) manifest.Source {
	return manifest.Source{
		Name:      "Everloot (https://everloot.app), corroborated drop rates",
		Version:   s.DataAsOf,
		Retrieved: s.Retrieved,
		License:   everloot.License + ". " + everloot.Credit,
	}
}

func writeTable[T any](outDir, name string, rows []T, opts parquetio.Options) (manifest.Table, error) {
	n, sum, err := parquetio.WriteFile(filepath.Join(outDir, name), rows, opts)
	return manifest.Table{Path: name, Rows: n, SHA256: sum, SortKeys: opts.Sort, Sources: opts.Sources}, err
}

// BuildDataset writes items, quests, npcs and loot_sources Parquet files and
// manifest.json into outDir. everlootFile is the cached Everloot snapshot.
func BuildDataset(cacheBuildDir, everlootFile, outDir, created string) (manifest.Manifest, error) {
	c, err := LoadCache(cacheBuildDir)
	if err != nil {
		return manifest.Manifest{}, err
	}
	ev, err := everloot.Load(everlootFile)
	if err != nil {
		return manifest.Manifest{}, err
	}
	src := SourceFrom(c.Prov)
	byID := Layout(nil, []string{"id"}, src) // no Bloom filters: see docs/spikes/parquet-layout.md
	m := manifest.Manifest{DatasetVersion: c.Prov.Build, GameBuild: c.Prov.Build, Created: created}
	add := func(t manifest.Table, err error) error {
		m.Tables = append(m.Tables, t)
		return err
	}
	if err := add(writeTable(outDir, "items.parquet", c.Items, byID)); err != nil {
		return m, err
	}
	if err := add(writeTable(outDir, "quests.parquet", c.Quests, byID)); err != nil {
		return m, err
	}
	if err := add(writeTable(outDir, "npcs.parquet", c.Npcs, byID)); err != nil {
		return m, err
	}
	if err := add(writeTable(outDir, "talent_trees.parquet", c.TalentTrees, Layout(nil, []string{"tree_id", "group_id"}, src))); err != nil {
		return m, err
	}
	if err := add(writeTable(outDir, "talent_nodes.parquet", c.TalentNodes, Layout(nil, []string{"node_id"}, src))); err != nil {
		return m, err
	}
	if err := add(writeTable(outDir, "stat_types.parquet", StatTypeTable(), Layout(nil, []string{"id"}, src))); err != nil {
		return m, err
	}
	if err := add(writeTable(outDir, "recipes.parquet", c.Recipes, Layout(nil, []string{"skill_line_id", "spell_id"}, src))); err != nil {
		return m, err
	}
	if err := add(writeTable(outDir, "recipe_reagents.parquet", c.RecipeReagents, Layout(nil, []string{"spell_id", "reagent_item_id"}, src))); err != nil {
		return m, err
	}
	if err := add(writeTable(outDir, "instances.parquet", c.Instances, Layout(nil, []string{"map_id"}, src))); err != nil {
		return m, err
	}
	if err := add(writeTable(outDir, "instance_bosses.parquet", c.InstanceBosses, Layout(nil, []string{"map_id", "order_index", "encounter_id"}, src))); err != nil {
		return m, err
	}
	loot := Layout(nil, []string{"item_id", "source_kind", "source_id"}, EverlootSource(ev))
	if err := add(writeTable(outDir, "loot_sources.parquet", everloot.BuildLootSources(ev), loot)); err != nil {
		return m, err
	}
	return m, manifest.Write(m, filepath.Join(outDir, "manifest.json"))
}
