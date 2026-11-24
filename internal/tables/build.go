package tables

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/WoW-Companion/data/internal/everloot"
	"github.com/WoW-Companion/data/internal/fetch"
	"github.com/WoW-Companion/data/internal/manifest"
	"github.com/WoW-Companion/data/internal/names"
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
	Prov       fetch.Provenance
	Items      []Item
	Quests     []Quest
	ItemSets   []ItemSetRow
	QuestLines []QuestLineRow
	Npcs       []Npc
	UiMaps     []Row

	TalentTrees []TalentTree
	TalentNodes []TalentNode

	Recipes        []Recipe
	RecipeReagents []RecipeReagent

	ClassSpells []ClassSpell

	Instances      []Instance
	InstanceBosses []InstanceBoss

	Nodes map[int32]Node
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
	c.ItemSets = BuildItemSets(rows["ItemSet"])
	c.QuestLines = BuildQuestLines(rows["QuestLine"], rows["QuestLineXQuest"])
	c.Npcs = BuildNpcs(rows["Creature"], rows["CreatureDifficulty"])
	c.UiMaps = rows["UiMap"]
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
	c.ClassSpells = BuildClassSpells(ClassSpellInput{
		SkillLines: rows["SkillLine"], Abilities: rows["SkillLineAbility"], SkillClasses: rows["SkillRaceClassInfo"],
		Classes: rows["ChrClasses"], SpellNames: rows["SpellName"], Spells: rows["Spell"], SpellLevels: rows["SpellLevels"],
	})
	iin := InstanceInput{
		Maps: rows["Map"], Encounters: rows["DungeonEncounter"], MapDifficulties: rows["MapDifficulty"],
		ContentTuning: rows["ContentTuning"],
	}
	c.Instances = BuildInstances(iin)
	c.InstanceBosses = BuildInstanceBosses(c.Instances, iin)
	c.Nodes = BuildNodes(GatherInput{GameObjects: rows["GameObjects"], Locks: rows["Lock"], LockTypes: rows["LockType"]})
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

// CorpusSource is the manifest source of quests_observed and vendor_sources, carrying the
// Everloot attribution.
// An absent corpus cache (no rows) takes the drop snapshot's dates, which the manifest needs.
func CorpusSource(c everloot.Corpus, drops everloot.Snapshot) manifest.Source {
	if c.DataAsOf == "" || c.Retrieved == "" {
		c.DataAsOf, c.Retrieved = drops.DataAsOf, drops.Retrieved
	}
	return manifest.Source{Name: everloot.CorpusName, Version: c.DataAsOf, Retrieved: c.Retrieved, License: everloot.License + ". " + everloot.Credit}
}

func writeTable[T any](outDir, name string, rows []T, opts parquetio.Options) (manifest.Table, error) {
	n, sum, err := parquetio.WriteFile(filepath.Join(outDir, name), rows, opts)
	return manifest.Table{Path: name, Rows: n, SHA256: sum, SortKeys: opts.Sort, Sources: opts.Sources}, err
}

// CuratedSource is the manifest source of the curated YAML under curated/.
func CuratedSource(build, created string) manifest.Source {
	return manifest.Source{Name: BuildSource, Version: build, Retrieved: created, License: "CC BY-SA 4.0 (LICENSE-DATA)"}
}

// BuildDataset writes items, quests, npcs and loot_sources Parquet files and
// manifest.json into outDir. everlootFile is the cached Everloot snapshot; talentsDir holds
// the curated talent builds, which are validated against the cached talent tables.
func BuildDataset(cacheBuildDir, everlootFile, talentsDir, outDir, created, revision string) (manifest.Manifest, error) {
	c, err := LoadCache(cacheBuildDir)
	if err != nil {
		return manifest.Manifest{}, err
	}
	ev, err := everloot.Load(everlootFile)
	if err != nil {
		return manifest.Manifest{}, err
	}
	corpus, err := everloot.LoadCorpus(filepath.Join(filepath.Dir(everlootFile), everloot.CorpusFile))
	if err != nil {
		return manifest.Manifest{}, err
	}
	quests := everloot.BuildQuestsObserved(corpus)
	vendors := everloot.BuildVendorSources(corpus)
	// loot_sources before npcs: the rows decide which Everloot-only creatures npcs adds.
	rows := MergeLootSources(everloot.BuildLootSources(ev), BuildCraftedSources(c.Recipes))
	rows = everloot.MergeNewKeys(rows, everloot.BuildQuestRewards(corpus))
	rows = everloot.MergeNewKeys(rows, everloot.BuildSoldBy(corpus, vendors))
	creatures, clientOnly, _ := ZoneCoverage(c.Npcs, rows)
	c.Npcs = AddEverlootNpcs(c.Npcs, rows, everloot.BuildNpcNames(ev.Rates))
	ApplyNpcZones(c.Npcs, c.UiMaps, everloot.BuildNpcZones(ev.NpcZones))
	linked := LinkBosses(c.InstanceBosses, c.Npcs)
	builds, err := LoadTalentBuilds(talentsDir)
	if err != nil {
		return manifest.Manifest{}, err
	}
	buildRows, err := BuildTalentBuilds(builds, c.TalentTrees, c.TalentNodes)
	if err != nil {
		return manifest.Manifest{}, err
	}
	src := SourceFrom(c.Prov)
	byID := Layout(nil, []string{"id"}, src) // no Bloom filters: see docs/spikes/parquet-layout.md
	m := manifest.Manifest{DatasetVersion: c.Prov.Build, GameBuild: c.Prov.Build, Created: created, Revision: revision}
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
	if err := add(writeTable(outDir, "npcs.parquet", c.Npcs, Layout(nil, []string{"id"}, src, EverlootSource(ev)))); err != nil {
		return m, err
	}
	if err := add(writeTable(outDir, "talent_trees.parquet", c.TalentTrees, Layout(nil, []string{"tree_id", "group_id"}, src))); err != nil {
		return m, err
	}
	if err := add(writeTable(outDir, "talent_nodes.parquet", c.TalentNodes, Layout(nil, []string{"node_id"}, src))); err != nil {
		return m, err
	}
	if err := add(writeTable(outDir, "talent_builds.parquet", buildRows, Layout(nil, []string{"class", "spec", "order"}, CuratedSource(c.Prov.Build, created), src))); err != nil {
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
	if err := add(writeTable(outDir, "class_spells.parquet", c.ClassSpells, Layout(nil, []string{"class_id", "level", "spell_id", "skill_line_id"}, src))); err != nil {
		return m, err
	}
	if err := add(writeTable(outDir, "instances.parquet", c.Instances, Layout(nil, []string{"map_id"}, src))); err != nil {
		return m, err
	}
	if err := add(writeTable(outDir, "instance_bosses.parquet", c.InstanceBosses, Layout(nil, []string{"map_id", "order_index", "encounter_id"}, src))); err != nil {
		return m, err
	}
	loot := Layout(nil, []string{"item_id", "source_kind", "source_id"}, EverlootSource(ev), src)
	gathered := ApplyGathered(rows, c.Nodes)
	fmt.Printf("gathering nodes in the client tables: %d, gathered loot rows: %d\n", len(c.Nodes), gathered)
	_, inNpcs, withZone := ZoneCoverage(c.Npcs, rows)
	fmt.Printf("creature loot sources: %d, resolved in npcs: %d (%.1f%%; client rows alone %d, %.1f%%), with a zone: %d (%.1f%%)\n",
		creatures, inNpcs, pctOf(inNpcs, creatures), clientOnly, pctOf(clientOnly, creatures), withZone, pctOf(withZone, creatures))
	fmt.Printf("instance bosses linked to a creature: %d of %d (%.1f%%)\n", linked, len(c.InstanceBosses), pctOf(linked, len(c.InstanceBosses)))
	if err := add(writeTable(outDir, "loot_sources.parquet", rows, loot)); err != nil {
		return m, err
	}
	corpusSrc := CorpusSource(corpus, ev)
	if err := add(writeTable(outDir, "quests_observed.parquet", quests, Layout(nil, []string{"quest_id"}, corpusSrc))); err != nil {
		return m, err
	}
	if err := add(writeTable(outDir, "vendor_sources.parquet", vendors, Layout(nil, []string{"npc_id", "item_id"}, corpusSrc))); err != nil {
		return m, err
	}
	fmt.Printf("observed quests: %d, vendor offers: %d\n", len(quests), len(vendors))
	if err := add(writeTable(outDir, "item_sets.parquet", c.ItemSets, Layout(nil, []string{"set_id", "item_id"}, src))); err != nil {
		return m, err
	}
	if err := add(writeTable(outDir, "quest_lines.parquet", c.QuestLines, Layout(nil, []string{"line_id", "position", "quest_id"}, src))); err != nil {
		return m, err
	}
	// names.parquet: the prebuilt name index. One row group per DefaultRowGroup rows
	// (docs/layout.md has the measurements) and a Bloom filter on norm for exact names.
	nameRows := NameIndex(c)
	if err := add(writeTable(outDir, names.File, nameRows, Layout([]string{"norm"}, names.SortKeys, src))); err != nil {
		return m, err
	}
	return m, manifest.Write(m, filepath.Join(outDir, "manifest.json"))
}

func pctOf(n, total int) float64 {
	if total == 0 {
		return 0
	}
	return 100 * float64(n) / float64(total)
}
