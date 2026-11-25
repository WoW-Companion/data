package tables

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/WoW-Companion/data/internal/everloot"
	"github.com/WoW-Companion/data/internal/fetch"
	"github.com/WoW-Companion/data/internal/manifest"
	"github.com/WoW-Companion/data/internal/names"
	"github.com/WoW-Companion/data/internal/parquetio"
	"github.com/parquet-go/parquet-go"
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
	RecipeItems    []RecipeItem
	SkillLineNames []string

	ClassSpells []ClassSpell

	Instances      []Instance
	InstanceBosses []InstanceBoss
	InstanceQuests []InstanceQuest

	Nodes map[int32]Node

	// RandomSuffixes come from the cache of fetch.RandomSuffixBuild, a sibling of the
	// build's cache directory; RandomSuffixProv is zero when that cache is absent.
	RandomSuffixes   []RandomSuffix
	RandomSuffixProv fetch.Provenance
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
	c.Items = BuildItems(rows["ItemSparse"], rows["Item"], NewBudgets(rows))
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
	c.RecipeItems = BuildRecipeItems(c.Recipes, RecipeItemInput{ItemEffects: rows["ItemEffect"], ItemLinks: rows["ItemXItemEffect"], SpellEffects: rows["SpellEffect"], Items: rows["ItemSparse"]})
	c.SkillLineNames = ProfessionLineNames(rows["SkillLine"])
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
	if err := loadRandomSuffixes(&c, filepath.Join(filepath.Dir(dir), fetch.RandomSuffixBuild)); err != nil {
		return c, err
	}
	return c, nil
}

// loadRandomSuffixes reads the random suffix tables from their own cache directory; an
// absent directory leaves them empty (the build then carries the earlier release's rows).
func loadRandomSuffixes(c *Cached, dir string) error {
	b, err := os.ReadFile(filepath.Join(dir, fetch.ProvenanceFile))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := json.Unmarshal(b, &c.RandomSuffixProv); err != nil {
		return err
	}
	rows := map[string][]Row{}
	for _, t := range fetch.RandomSuffixTables {
		if rows[t], err = readCSVFile(dir, t); err != nil {
			return err
		}
	}
	c.RandomSuffixes = BuildRandomSuffixes(RandomSuffixInput{Properties: rows["ItemRandomProperties"], Suffixes: rows["ItemRandomSuffix"], Enchants: rows["SpellItemEnchantment"]})
	return nil
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

// everLicense is the licence and credit text of an Everloot source; a source read from the
// data files export also carries the Warcraft Wiki name credit.
func everLicense(origin string) string {
	l := everloot.License + ". " + everloot.Credit
	if strings.HasPrefix(origin, "export") {
		l += " " + everloot.WikiCredit
	}
	return l
}

// EverlootSource turns a cached Everloot snapshot into a manifest source carrying its
// attribution.
func EverlootSource(s everloot.Snapshot) manifest.Source {
	return manifest.Source{
		Name:      "Everloot (https://everloot.app), corroborated drop rates",
		Version:   everloot.SourceVersion(s.Origin, s.DataAsOf),
		Retrieved: s.Retrieved,
		License:   everLicense(s.Origin),
	}
}

// CorpusSource is the manifest source of quests_observed and vendor_sources, carrying the
// Everloot attribution.
// An absent corpus cache (no rows) takes the drop snapshot's dates, which the manifest needs.
func CorpusSource(c everloot.Corpus, drops everloot.Snapshot) manifest.Source {
	if c.DataAsOf == "" || c.Retrieved == "" {
		c.DataAsOf, c.Retrieved, c.Origin = drops.DataAsOf, drops.Retrieved, drops.Origin
	}
	return manifest.Source{Name: everloot.CorpusName, Version: everloot.SourceVersion(c.Origin, c.DataAsOf), Retrieved: c.Retrieved, License: everLicense(c.Origin)}
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
// the curated talent builds, which are validated against the cached talent tables, and
// instancesFile the curated level ranges and entrance zones.
func BuildDataset(cacheBuildDir, everlootFile, talentsDir, instancesFile, professionsFile, outDir, created, revision string) (manifest.Manifest, error) {
	return BuildDatasetFrom(cacheBuildDir, everlootFile, talentsDir, instancesFile, professionsFile, outDir, created, revision, nil)
}

// Previous gives the rows of an earlier release. Table returns the parquet file of a table
// and the manifest source to record for it, or false when no release holds it with rows.
type Previous interface {
	Table(name string) ([]byte, manifest.Source, bool)
}

// carried returns rows unchanged unless their Everloot source was unavailable in this
// build; then it takes them from prev (and the source to record), when prev has them.
func carried[T any](prev Previous, table string, unavailable bool, rows []T, src manifest.Source) ([]T, manifest.Source) {
	if !unavailable || prev == nil {
		return rows, src
	}
	b, psrc, ok := prev.Table(table)
	if !ok {
		fmt.Printf("carry forward %s: no earlier release has it, built empty\n", table)
		return rows, src
	}
	got, err := parquet.Read[T](bytes.NewReader(b), int64(len(b)))
	if err != nil || len(got) == 0 {
		fmt.Printf("carry forward %s: earlier rows unreadable (%v), built empty\n", table, err)
		return rows, src
	}
	fmt.Printf("carry forward %s: %d rows from %s\n", table, len(got), psrc.Name)
	return got, psrc
}

// BuildDatasetFrom is BuildDataset with prev: an Everloot source that could not be fetched
// takes its tables from prev's newest release that has them, instead of building empty.
func BuildDatasetFrom(cacheBuildDir, everlootFile, talentsDir, instancesFile, professionsFile, outDir, created, revision string, prev Previous) (manifest.Manifest, error) {
	c, err := LoadCache(cacheBuildDir)
	if err != nil {
		return manifest.Manifest{}, err
	}
	ev, err := everloot.Load(everlootFile)
	if os.IsNotExist(err) {
		ev, err = everloot.Snapshot{}, nil // the fetch could not reach Everloot: built empty
	}
	if err == nil && ev.Retrieved == "" {
		// An empty source still names a retrieval time and version in the manifest.
		ev.Retrieved, ev.DataAsOf = created, "unavailable"
	}
	if err != nil {
		return manifest.Manifest{}, err
	}
	corpus, err := everloot.LoadCorpus(filepath.Join(filepath.Dir(everlootFile), everloot.CorpusFile))
	if err != nil {
		return manifest.Manifest{}, err
	}
	disenchant, err := everloot.LoadDisenchant(filepath.Join(filepath.Dir(everlootFile), everloot.DisenchantFile))
	if err != nil {
		return manifest.Manifest{}, err
	}
	profNotes, err := LoadCuratedProfessions(professionsFile, c.SkillLineNames)
	if err != nil {
		return manifest.Manifest{}, err
	}
	corpusGone := corpus.DataAsOf == "" || corpus.Retrieved == ""
	deGone := disenchant.DataAsOf == "" || disenchant.Retrieved == ""
	corpusSrc := CorpusSource(corpus, ev)
	quests, qs := carried(prev, "quests_observed.parquet", corpusGone, everloot.BuildQuestsObserved(corpus), corpusSrc)
	vendors, vs := carried(prev, "vendor_sources.parquet", corpusGone, everloot.BuildVendorSources(corpus), corpusSrc)
	// loot_sources before npcs: the rows decide which Everloot-only creatures npcs adds.
	rows := MergeLootSources(everloot.BuildLootSources(ev), BuildCraftedSources(c.Recipes))
	rows = everloot.MergeNewKeys(rows, everloot.BuildQuestRewards(corpus))
	rows = everloot.MergeNewKeys(rows, everloot.BuildSoldBy(corpus, vendors))
	creatures, clientOnly, _ := ZoneCoverage(c.Npcs, rows)
	c.Npcs = AddEverlootNpcs(c.Npcs, rows, everloot.BuildNpcNames(ev.Rates))
	ApplyNpcZones(c.Npcs, c.UiMaps, everloot.BuildNpcZones(ev.NpcZones))
	hasLoot := map[int32]bool{}
	for _, r := range rows {
		if r.SourceKind == everloot.KindCreature {
			hasLoot[r.SourceID] = true
		}
	}
	trainers, trainerSpells := BuildTrainers(everloot.BuildTrainerOffers(corpus), c.Recipes, c.Npcs, everloot.BuildNpcZones(ev.NpcZones), c.UiMaps, c.SkillLineNames)
	trainers, ts := carried(prev, "trainers.parquet", corpusGone, trainers, corpusSrc)
	trainerSpells, tss := carried(prev, "trainer_spells.parquet", corpusGone, trainerSpells, corpusSrc)
	nodeObs, nodeYields := everloot.BuildGatherObs(corpus)
	gatherNodes, gatherYields := BuildGathering(nodeObs, nodeYields, c.Nodes, c.UiMaps, rows)
	gatherNodes, gns := carried(prev, "gathering_nodes.parquet", corpusGone, gatherNodes, corpusSrc)
	gatherYields, gys := carried(prev, "gathering_yields.parquet", corpusGone, gatherYields, corpusSrc)
	fishing, fs := carried(prev, "fishing_catches.parquet", corpusGone, BuildFishingCatches(everloot.BuildFishing(corpus), c.UiMaps), corpusSrc)
	if disenchant.DataAsOf == "" || disenchant.Retrieved == "" {
		disenchant.DataAsOf, disenchant.Retrieved = ev.DataAsOf, ev.Retrieved
	}
	deSrc := manifest.Source{Name: "Everloot (https://everloot.app), disenchant rollup", Version: disenchant.DataAsOf, Retrieved: disenchant.Retrieved, License: everloot.License + ". " + everloot.Credit}
	deYields, deSrc := carried(prev, "disenchant_yields.parquet", deGone, everloot.BuildDisenchantYields(disenchant), deSrc)
	fmt.Printf("everloot names dropped for not being English: %d\n", everloot.DroppedNames.Load())
	byTrainer, byItem, byBoth := ApplyLearnedFrom(c.Recipes, c.RecipeItems, TrainedSpells(trainerSpells))
	ls := ApplyLearnSkills(c.Recipes, c.RecipeItems, trainerSpells)
	SyncCraftedLearnSkill(rows, c.Recipes)
	fmt.Printf("learn skill: estimate k=%g fitted on %d recipes, error mean %.1f median %.0f p90 %.0f; %d above yellow\n", ls.K, ls.Known, ls.MeanErr, ls.MedianErr, ls.P90Err, len(ls.AboveYellow))
	fmt.Print(ls.Report())
	fmt.Printf("recipes learned from: trainer %d, item %d, both %d; recipe items %d, trainers %d, trainer offers %d\n", byTrainer, byItem, byBoth, len(c.RecipeItems), len(trainers), len(trainerSpells))
	fmt.Printf("gathering nodes %d, yields %d, fishing catches %d, disenchant yields %d\n", len(gatherNodes), len(gatherYields), len(fishing), len(deYields))
	linked := LinkBosses(c.InstanceBosses, c.Npcs, hasLoot)
	curated, err := LoadCuratedInstances(instancesFile)
	if err != nil {
		return manifest.Manifest{}, err
	}
	if err := ApplyInstanceData(c.Instances, curated, c.UiMaps, everloot.BuildEntranceZones(corpus)); err != nil {
		return manifest.Manifest{}, err
	}
	corroborated := ApplyEncounterObs(c.InstanceBosses, everloot.BuildEncounterObs(corpus))
	bossLoot := TagInstanceLoot(rows, c.InstanceBosses)
	c.InstanceQuests = BuildInstanceQuests(c.Instances, quests, c.Quests)
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
	if err := add(writeTable(outDir, "instances.parquet", c.Instances, Layout(nil, []string{"map_id"}, src, CuratedSource(c.Prov.Build, created), CorpusSource(corpus, ev)))); err != nil {
		return m, err
	}
	if err := add(writeTable(outDir, "instance_bosses.parquet", c.InstanceBosses, Layout(nil, []string{"map_id", "order_index", "encounter_id"}, src, CorpusSource(corpus, ev)))); err != nil {
		return m, err
	}
	if err := add(writeTable(outDir, "instance_quests.parquet", c.InstanceQuests, Layout(nil, []string{"map_id", "quest_id", "reason"}, CorpusSource(corpus, ev), src))); err != nil {
		return m, err
	}
	loot := Layout(nil, []string{"item_id", "source_kind", "source_id"}, EverlootSource(ev), src)
	gathered := ApplyGathered(rows, c.Nodes)
	fmt.Printf("gathering nodes in the client tables: %d, gathered loot rows: %d\n", len(c.Nodes), gathered)
	_, inNpcs, withZone := ZoneCoverage(c.Npcs, rows)
	fmt.Printf("creature loot sources: %d, resolved in npcs: %d (%.1f%%; client rows alone %d, %.1f%%), with a zone: %d (%.1f%%)\n",
		creatures, inNpcs, pctOf(inNpcs, creatures), clientOnly, pctOf(clientOnly, creatures), withZone, pctOf(withZone, creatures))
	fmt.Printf("instances: %s\n", InstanceCoverage(c.Instances, c.InstanceBosses, rows, c.InstanceQuests, corroborated, bossLoot))
	fmt.Printf("instance bosses linked to a creature: %d of %d (%.1f%%)\n", linked, len(c.InstanceBosses), pctOf(linked, len(c.InstanceBosses)))
	if err := add(writeTable(outDir, "loot_sources.parquet", rows, loot)); err != nil {
		return m, err
	}
	if err := add(writeTable(outDir, "quests_observed.parquet", quests, Layout(nil, []string{"quest_id"}, qs))); err != nil {
		return m, err
	}
	if err := add(writeTable(outDir, "vendor_sources.parquet", vendors, Layout(nil, []string{"npc_id", "item_id"}, vs))); err != nil {
		return m, err
	}
	fmt.Printf("observed quests: %d, vendor offers: %d\n", len(quests), len(vendors))
	if err := add(writeTable(outDir, "item_sets.parquet", c.ItemSets, Layout(nil, []string{"set_id", "item_id"}, src))); err != nil {
		return m, err
	}
	suffixes, sfs := c.RandomSuffixes, src
	if c.RandomSuffixProv.Build != "" {
		sfs = SourceFrom(c.RandomSuffixProv)
	}
	suffixes, sfs = carried(prev, "random_suffixes.parquet", len(suffixes) == 0, suffixes, sfs)
	if err := add(writeTable(outDir, "random_suffixes.parquet", suffixes, Layout(nil, []string{"kind", "suffix_id", "name"}, sfs))); err != nil {
		return m, err
	}
	if err := add(writeTable(outDir, "quest_lines.parquet", c.QuestLines, Layout(nil, []string{"line_id", "position", "quest_id"}, src))); err != nil {
		return m, err
	}
	profSrc := CuratedSource(c.Prov.Build, created)
	if err := add(writeTable(outDir, "recipe_items.parquet", c.RecipeItems, Layout(nil, []string{"item_id", "teaches_spell_id"}, src))); err != nil {
		return m, err
	}
	if err := add(writeTable(outDir, "trainers.parquet", trainers, Layout(nil, []string{"npc_id", "profession"}, ts, src))); err != nil {
		return m, err
	}
	if err := add(writeTable(outDir, "trainer_spells.parquet", trainerSpells, Layout(nil, []string{"npc_id", "profession", "recipe_name"}, tss, src))); err != nil {
		return m, err
	}
	if err := add(writeTable(outDir, "gathering_nodes.parquet", gatherNodes, Layout(nil, []string{"node_id", "zone_id"}, gns, src))); err != nil {
		return m, err
	}
	if err := add(writeTable(outDir, "gathering_yields.parquet", gatherYields, Layout(nil, []string{"node_id", "item_id"}, gys))); err != nil {
		return m, err
	}
	if err := add(writeTable(outDir, "fishing_catches.parquet", fishing, Layout(nil, []string{"zone_id", "item_id"}, fs))); err != nil {
		return m, err
	}
	if err := add(writeTable(outDir, "disenchant_yields.parquet", deYields, Layout(nil, []string{"band_id", "material_item_id"}, deSrc))); err != nil {
		return m, err
	}
	if err := add(writeTable(outDir, "profession_notes.parquet", profNotes.Rows, Layout(nil, []string{"section"}, profSrc))); err != nil {
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
