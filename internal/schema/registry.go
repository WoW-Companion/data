// Package schema is the one registry of the dataset's tables: row type, file, source and
// the meaning of every column. `wowc-data schema` renders docs/schema.md from it and
// `wowc-data coverage` reads a built dataset against it.
package schema

import (
	"github.com/WoW-Companion/data/internal/everloot"
	"github.com/WoW-Companion/data/internal/tables"
)

// Col documents one column. Source is empty when the column comes from the table's source.
type Col struct {
	Name, Source, Meaning string
}

// Table is one published Parquet file.
type Table struct {
	Name    string
	Row     any // the row struct whose parquet tags define the columns
	Source  string
	Meaning string
	Cols    []Col
}

// File is the table's file name in a dataset directory.
func (t Table) File() string { return t.Name + ".parquet" }

const (
	wago   = "wago.tools client table "
	curate = "curated YAML (curated/talents)"
)

// Tables lists every table in the order BuildDataset writes them.
var Tables = []Table{
	{"items", tables.Item{}, wago + "ItemSparse, Item", "One row per item the client lists, sorted by id.", []Col{
		{"id", "", "Item id."},
		{"name", "", "Item name (ItemSparse Display_lang)."},
		{"item_level", "", "Item level."},
		{"required_level", "", "Character level needed to equip or use it."},
		{"quality", "", "Quality id (0 poor, 1 common, 2 uncommon, 3 rare, 4 epic, 5 legendary)."},
		{"class_id", "Item.ClassID", "Item class id (2 weapon, 4 armor)."},
		{"subclass_id", "Item.SubclassID", "Item subclass id within the class."},
		{"slot", "", "Inventory type id (0 means not equippable)."},
		{"allowable_class_mask", "", "Bit mask of classes that may use it; -1 means all."},
		{"bonding", "", "Bonding id (0 none, 1 on pickup, 2 on equip, 3 on use)."},
		{"stat_types", "", "Stat type ids the item lists, in slot order; empty when none (see stat_types)."},
		{"stats_status", "derived", "known when stats are listed, unknown when a stat-bearing piece lists none, none otherwise."},
		{"stats", "", "One {type, value} per listed stat; value is always null, the client holds no amount."},
		{"armor", "", "Always null: the client holds no per-item armor amount."},
		{"dmg_min", "", "Always null: the client holds no per-item damage amount."},
		{"dmg_max", "", "Always null: the client holds no per-item damage amount."},
		{"speed_ms", "", "Weapon speed in milliseconds (ItemDelay); null for non-weapons or a zero delay."},
	}},
	{"quests", tables.Quest{}, wago + "QuestV2, QuestLineXQuest, QuestPOIBlob, UiMap", "One row per quest the client lists, sorted by id.", []Col{
		{"id", "", "Quest id."},
		{"unique_bit_flag", "", "Client unique-quest bit flag."},
		{"ui_theme_id", "", "Quest details UI theme id."},
		{"quest_line_id", "QuestLineXQuest", "First quest line the quest belongs to; null when none."},
		{"map_id", "QuestPOIBlob", "Map of the quest's first point of interest; null when none."},
		{"ui_map_id", "QuestPOIBlob", "UiMap of the quest's first point of interest; null when none."},
		{"level", "none", "Always null: no allowed source holds a quest level."},
		{"min_level", "none", "Always null: no allowed source holds a minimum level."},
		{"zone_id", "QuestPOIBlob", "UiMap id of the first point of interest when non-zero; null otherwise."},
		{"zone_name", "UiMap", "Name of that UiMap; null when the UiMap table has no named row."},
	}},
	{"npcs", tables.Npc{}, wago + "Creature, CreatureDifficulty", "One row per creature, sorted by id.", []Col{
		{"id", "", "Creature id."},
		{"name", "", "Creature name."},
		{"classification", "", "Client value: 0 normal, 1 elite, 2 rare elite, 3 boss, 4 rare."},
		{"creature_type", "", "Creature type id."},
		{"family_id", "", "Creature family id."},
		{"faction_template_id", "CreatureDifficulty", "Faction template id; null when the creature has no difficulty row."},
		{"min_level", "CreatureDifficulty", "Lowest level; null when the creature has no difficulty row."},
		{"max_level", "CreatureDifficulty", "Highest level; null when the creature has no difficulty row."},
	}},
	{"talent_trees", tables.TalentTree{}, wago + "TraitNodeGroup, TraitCond, SkillLineXTraitTree, SkillRaceClassInfo, ChrClasses", "One row per talent group of a trait tree.", []Col{
		{"tree_id", "", "Trait tree id."},
		{"class_id", "SkillLineXTraitTree, SkillRaceClassInfo", "Class id of the tree; null for a tree no skill line ties to a class."},
		{"class_name", "ChrClasses", "Class name; null with class_id."},
		{"group_id", "", "Trait node group id."},
		{"group_name", "none", "Always null: no client table names a group."},
		{"group_order", "derived", "Position of the group in its tree by ascending group id, from 0."},
		{"required_points", "TraitCond", "Points to spend in the tree before the group opens; null when no condition is listed."},
	}},
	{"talent_nodes", tables.TalentNode{}, wago + "TraitNode and the trait entry, definition, edge and SpellName tables", "One row per talent node.", []Col{
		{"node_id", "", "Trait node id."},
		{"tree_id", "", "Trait tree id."},
		{"group_ids", "TraitNodeGroupXTraitNode", "Ids of every group the node sits in."},
		{"spell_id", "TraitDefinition", "Spell of the node's first entry; null when it has none."},
		{"name", "TraitDefinition, SpellName", "Definition override name, else the spell's name; null when neither exists."},
		{"max_rank", "TraitNodeEntry", "Ranks of the node's first entry; null when it has none."},
		{"prerequisite_node_ids", "TraitEdge", "Left-hand node of each edge pointing at the node, whatever the edge type."},
	}},
	{"talent_builds", tables.TalentBuildRow{}, curate + " validated against talent_nodes", "Curated talent builds: the nodes to take, in order, per class, spec and level band.", []Col{
		{"class", "", "Class name."},
		{"spec", "", "Specialisation name."},
		{"band", "", "Level band the step belongs to, low-high."},
		{"order", "", "Step number within the spec, from 1, across all bands."},
		{"node_id", "", "talent_nodes.node_id to spend points in."},
		{"points", "", "Ranks to add at this step."},
		{"rationale", "", "Short reason for the band's choices."},
		{"source", "", "Always \"" + tables.BuildSource + "\"."},
	}},
	{"stat_types", tables.StatTypeRow{}, "constants in internal/tables/itemstats.go", "Labels for the item stat ids 0 to 56.", []Col{
		{"id", "", "Stat type id, as in items.stat_types."},
		{"label", "", "Short label of the client's ITEM_MOD enum."},
	}},
	{"recipes", tables.Recipe{}, wago + "SkillLine, SkillLineAbility, SpellReagents, SpellEffect, SpellName", "One row per profession recipe, once per skill line and spell.", []Col{
		{"spell_id", "", "Recipe spell id."},
		{"skill_line_id", "", "Skill line (profession) id."},
		{"skill_line_name", "SkillLine", "Profession name."},
		{"name", "SpellName", "Spell name; null when SpellName has none."},
		{"creates_item_id", "SpellEffect", "Item the recipe makes; null when it makes none."},
		{"learn_skill", "SkillLineAbility", "Minimum skill rank to learn it; null when the client stores zero."},
		{"yellow_skill", "SkillLineAbility", "Skill rank at which it turns yellow; null when zero."},
		{"grey_skill", "SkillLineAbility", "Skill rank at which it turns grey; null when zero."},
	}},
	{"recipe_reagents", tables.RecipeReagent{}, wago + "SpellReagents", "One row per reagent of each recipe.", []Col{
		{"spell_id", "", "recipes.spell_id."},
		{"reagent_item_id", "", "Item id of the reagent."},
		{"count", "", "Units needed; a reagent in two slots is summed."},
	}},
	{"instances", tables.Instance{}, wago + "Map, MapDifficulty, ContentTuning", "One row per dungeon or raid map.", []Col{
		{"map_id", "", "Map id."},
		{"name", "", "Map name."},
		{"instance_type", "", "dungeon or raid, as the client types the map."},
		{"max_players", "", "Maximum players; null when zero."},
		{"entrance_map_id", "", "Map of the entrance (CorpseMapID); null when none."},
		{"entrance_x", "", "Entrance position x; null when none."},
		{"entrance_y", "", "Entrance position y; null when none."},
		{"min_level", "ContentTuning", "LFG minimum level; null when the tuning row holds none."},
		{"max_level", "ContentTuning", "LFG maximum level; null when the tuning row holds none."},
	}},
	{"instance_bosses", tables.InstanceBoss{}, wago + "DungeonEncounter, Creature", "One row per dungeon encounter of an instance.", []Col{
		{"map_id", "", "instances.map_id."},
		{"encounter_id", "", "Dungeon encounter id."},
		{"name", "", "Encounter name."},
		{"order_index", "", "Order of the encounter in the instance."},
		{"difficulty_id", "", "Difficulty id (0 on nearly all)."},
		{"creature_id", "derived from Creature", "npcs.id when exactly one creature carries the encounter's exact name; null otherwise."},
	}},
	{"loot_sources", everloot.LootSource{}, "Everloot drop rates (CC BY-SA 4.0) and recipes", "Where an item comes from: creature, object, quest reward and crafted rows.", []Col{
		{"item_id", "", "Item id."},
		{"source_kind", "", "creature, object, quest reward or crafted."},
		{"source_id", "", "Creature id, object id, quest id, or the recipe spell id for crafted rows."},
		{"kills", "Everloot", "Loot windows opened (not kills); 0 on quest reward and crafted rows."},
		{"drops", "Everloot", "Times the item dropped; 0 on quest reward and crafted rows."},
		{"rate", "Everloot", "drops / kills; 0 where there is no drop chance."},
		{"rate_low", "Everloot", "Lower bound of the 95% Wilson range; null on crafted rows."},
		{"rate_high", "Everloot", "Upper bound of the 95% Wilson range; null on crafted rows."},
		{"reporters", "Everloot", "Distinct installs that reported it; 0 on crafted rows."},
		{"tier", "Everloot", "reported, verified or confirmed; null on crafted rows."},
		{"everloot_data_as_of", "Everloot", "Everloot's dataAsOf for the snapshot; null on crafted rows."},
		{"profession", "recipes", "Crafted rows only: the recipe's profession; null otherwise."},
		{"learn_skill", "recipes", "Crafted rows only: skill rank to learn the recipe; null otherwise."},
	}},
}
