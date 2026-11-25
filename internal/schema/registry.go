// Package schema is the one registry of the dataset's tables: row type, file, source and
// the meaning of every column. `wowc-data schema` renders docs/schema.md from it and
// `wowc-data coverage` reads a built dataset against it.
package schema

import (
	"github.com/WoW-Companion/data/internal/everloot"
	"github.com/WoW-Companion/data/internal/names"
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
	wago             = "wago.tools client table "
	curate           = "curated YAML (curated/talents)"
	everlootTrainers = "Everloot trainer_seen records (CC BY-SA 4.0), UiMap. Data from Everloot (https://everloot.app), CC BY-SA 4.0."
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
		{"sell_price", "ItemSparse SellPrice", "Vendor sell price in copper; null when the client stores zero."},
		{"buy_price", "ItemSparse BuyPrice", "Vendor buy price in copper; null when the client stores zero."},
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
	{"npcs", tables.Npc{}, wago + "Creature, CreatureDifficulty, UiMap; Everloot located_in and drop source names", "One row per client creature plus one per loot_sources creature the client lacks and Everloot names, sorted by id. Join loot_sources (source_kind creature) on source_id = id for the name and zone of a drop.", []Col{
		{"id", "", "Creature id."},
		{"name", "", "Creature name."},
		{"classification", "Creature", "Client value: 0 normal, 1 elite, 2 rare elite, 3 boss, 4 rare; null on a row from Everloot."},
		{"creature_type", "Creature", "Creature type id; null on a row from Everloot."},
		{"family_id", "Creature", "Creature family id; null on a row from Everloot."},
		{"faction_template_id", "CreatureDifficulty", "Faction template id; null when the creature has no difficulty row."},
		{"min_level", "CreatureDifficulty", "Lowest level; null when the creature has no difficulty row."},
		{"max_level", "CreatureDifficulty", "Highest level; null when the creature has no difficulty row."},
		{"zone_id", "Everloot located_in", "UiMap id of the zone Everloot observed the creature in (most installs, lowest id on a tie); null when Everloot places it nowhere."},
		{"zone_name", "UiMap", "Name of that UiMap; null with zone_id or when UiMap has no named row."},
		{"zone_source", "", "\"everloot located_in\" when zone_id is set; null otherwise."},
		{"name_source", "Everloot", "\"everloot sourceName\" on a creature the client tables lack, named by Everloot's drop rows; null on a client row."},
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
		{"learn_skill", "trainer_spells, recipe_items, SkillLineAbility, derived", "Skill rank needed to learn the recipe, from learn_skill_source: the trainer's required skill, the teaching recipe item's, the client's minimum when above 1, else an estimate. Null only when no source and no yellow_skill exist."},
		{"learn_skill_source", "derived", "trainer, recipe_item, client or estimate: where learn_skill came from, in that order of preference. estimate is yellow_skill minus a multiple of the yellow-to-grey span, fitted on the other rows (docs/sources.md). Null when learn_skill is null."},
		{"yellow_skill", "SkillLineAbility", "Skill rank at which it turns yellow; null when zero."},
		{"grey_skill", "SkillLineAbility", "Skill rank at which it turns grey; null when zero."},
		{"learned_from", "derived", "trainer when no recipe item teaches the spell, item when one does, both when a recipe item and a corroborated Everloot trainer offer both do (see recipe_items and trainer_spells). trainer also covers a spell no trainer was observed offering: it is the absence of a recipe item, not proof of a trainer."},
	}},
	{"recipe_reagents", tables.RecipeReagent{}, wago + "SpellReagents", "One row per reagent of each recipe.", []Col{
		{"spell_id", "", "recipes.spell_id."},
		{"reagent_item_id", "", "Item id of the reagent."},
		{"count", "", "Units needed; a reagent in two slots is summed."},
	}},
	{"class_spells", tables.ClassSpell{}, wago + "SkillLine, SkillLineAbility, SkillRaceClassInfo, ChrClasses, SpellName, Spell, SpellLevels", "One row per class ability rank on a class skill line (Arms, Fury, Holy ...), once per class, skill line and spell. Filter on class_id and level for what a level unlocks. Talents, professions, pets and racials are not listed.", []Col{
		{"class_id", "SkillRaceClassInfo, SkillLineAbility", "Class id (ChrClasses.ID) of the skill line; an ability ClassMask, when set, narrows it."},
		{"spell_id", "SkillLineAbility", "Spell id of the ability rank."},
		{"name", "SpellName", "Spell name."},
		{"rank", "Spell NameSubtext", "Rank number when the spell's subtext reads \"Rank N\"; null otherwise."},
		{"subtext", "Spell NameSubtext", "The spell's subtext (for example \"Rank 2\"); null when empty."},
		{"level", "SpellLevels", "Level the spell becomes learnable: SpellLevel of the difficulty-0 row, BaseLevel when that is zero. Abilities with neither are not listed."},
		{"skill_line_id", "SkillLineAbility", "Class skill line id."},
		{"skill_line_name", "SkillLine", "Class skill line name (a specialisation tree, for example Arms)."},
		{"superseded_spell_id", "SkillLineAbility SupercedesSpell", "The previous rank this one replaces; null for a first rank. It may name a spell with no row here when that spell has no level."},
		{"trained", "none", "Always null: the client tables do not say whether a trainer teaches a spell or it is learned automatically."},
	}},
	{"instances", tables.Instance{}, wago + "Map, MapDifficulty, ContentTuning, UiMap; curated instances.yaml (advisor defaults); Everloot instance_entrance records (CC BY-SA 4.0)", "One row per dungeon or raid map. Level range: the client's content tuning where it holds one, else the curated typical range (an advisor default, not authoritative). Entrance zone: Everloot's observed zone where at least two installs agree and none differ, else the curated one.", []Col{
		{"map_id", "", "Map id."},
		{"name", "", "Map name."},
		{"instance_type", "", "dungeon or raid, as the client types the map."},
		{"max_players", "", "Maximum players; null when zero."},
		{"entrance_map_id", "", "Map of the entrance (CorpseMapID); null when none."},
		{"entrance_x", "", "Entrance position x; null when none."},
		{"entrance_y", "", "Entrance position y; null when none."},
		{"min_level", "ContentTuning, curated", "Recommended minimum level: the client's LFG level, else the curated typical range; null when neither has one."},
		{"max_level", "ContentTuning, curated", "Recommended maximum level: the client's LFG level, else the curated typical range; null when neither has one."},
		{"level_source", "derived", "Where min_level and max_level came from: client or curated (observed is reserved: no Everloot record carries an instance level); null with no range."},
		{"entrance_zone_id", "curated, Everloot", "UiMap id of the open-world zone the entrance stands in; null when unknown."},
		{"entrance_zone_name", "UiMap", "Name of that zone (UiMap Name_lang); null when unknown."},
		{"entrance_zone_source", "derived", "observed (Everloot, at least two installs agreeing) or curated; null when unknown."},
	}},
	{"instance_bosses", tables.InstanceBoss{}, wago + "DungeonEncounter; npcs", "One row per dungeon encounter of an instance.", []Col{
		{"map_id", "", "instances.map_id."},
		{"encounter_id", "", "Dungeon encounter id."},
		{"name", "", "Encounter name."},
		{"order_index", "", "Order of the encounter in the instance."},
		{"difficulty_id", "", "Difficulty id (0 on nearly all)."},
		{"creature_id", "derived from npcs", "npcs.id when exactly one npc carries the encounter's exact name (or, among several, exactly one that is a loot source); null otherwise. Join loot_sources on source_kind creature and source_id = creature_id."},
		{"reporters", "Everloot", "Distinct installs that killed the encounter (Everloot encounter records); null when fewer than two."},
	}},
	{"instance_quests", tables.InstanceQuest{}, wago + "QuestPOIBlob, UiMap; Everloot observed quests (CC BY-SA 4.0). Data from Everloot (https://everloot.app), CC BY-SA 4.0.", "Quests tied to an instance: the ones to pick up before you go. Sorted by map, quest and reason.", []Col{
		{"map_id", "", "instances.map_id."},
		{"quest_id", "", "quests.id or quests_observed.quest_id."},
		{"reason", "derived", "category: the quest's observed log heading names the instance; zone: the client's quest zone names it. A quest whose objective creature drops only in an instance is not derivable: no source holds quest objectives."},
	}},
	{"loot_sources", everloot.LootSource{}, "Everloot drop rates (CC BY-SA 4.0) and recipes", "Where an item comes from: creature, object, quest reward, sold by, crafted and gathered rows.", []Col{
		{"item_id", "", "Item id."},
		{"source_kind", "", "creature, object, quest reward, sold by, crafted or gathered."},
		{"source_id", "", "Creature id, object id (the node id on gathered rows), quest id, vendor creature id on sold by rows, or the recipe spell id for crafted rows."},
		{"kills", "Everloot", "Loot windows opened (not kills); 0 on quest reward, sold by and crafted rows."},
		{"drops", "Everloot", "Times the item dropped; 0 on quest reward, sold by and crafted rows."},
		{"rate", "Everloot", "drops / kills; 0 where there is no drop chance."},
		{"rate_low", "Everloot", "Lower bound of the 95% Wilson range; null on crafted rows."},
		{"rate_high", "Everloot", "Upper bound of the 95% Wilson range; null on crafted rows."},
		{"reporters", "Everloot", "Distinct installs that reported it; 0 on crafted rows."},
		{"tier", "Everloot", "reported, verified or confirmed on drop rows; settled or corroborated on quest reward and sold by rows from observed quests and vendors; null on crafted rows."},
		{"everloot_data_as_of", "Everloot", "Everloot's dataAsOf for the snapshot; null on crafted rows."},
		{"profession", "recipes, GameObjects, Lock, LockType", "Crafted rows: the recipe's profession. Gathered rows: Herbalism or Mining. Null otherwise."},
		{"learn_skill", "recipes", "Crafted rows only: the recipe's learn_skill (see recipes.learn_skill_source); null otherwise."},
		{"node_id", "GameObjects", "Gathered rows only: the node's game object id (equals source_id); null otherwise."},
		{"node_name", "GameObjects", "Gathered rows only: the node's name; null when the client holds none."},
		{"skill_required", "Lock", "Gathered rows only: the skill rank the node's lock needs; null otherwise."},
		{"map_id", "instance_bosses", "Creature rows only: the instance map when the creature is a linked boss (instance_bosses.creature_id) of exactly one instance; null otherwise."},
	}},
	{"quests_observed", everloot.QuestObserved{}, "Everloot observed quests (CC BY-SA 4.0). Data from Everloot (https://everloot.app), CC BY-SA 4.0.", "One row per quest players observed, kept when Everloot lists it as settled or at least two distinct installs reported its title, sorted by quest id. Includes Forever's own quests (ids from 90000) that the client tables lack. A field is null unless the quest is settled or at least two installs reported that value. Join quests.id or loot_sources (source_kind quest reward) on quest_id.", []Col{
		{"quest_id", "Everloot", "Quest id."},
		{"title", "Everloot", "Quest title as players saw it (the most reported spelling)."},
		{"level", "Everloot", "Quest level; null when unsettled and fewer than two installs agree."},
		{"giver_npc_id", "Everloot", "Creature id of the quest giver (npcs.id may lack it); null when unreported or uncorroborated."},
		{"turn_in_npc_id", "Everloot", "Creature id where the quest is handed in; null when unreported or uncorroborated (Everloot's records carried none at the 2026-11-24 survey)."},
		{"faction", "Everloot", "alliance or horde as reported; null when absent or uncorroborated."},
		{"category", "Everloot", "Zone or dungeon heading of the quest log; null when absent or uncorroborated."},
		{"suggested_group", "Everloot", "Suggested group size; null when absent or uncorroborated."},
		{"tag", "Everloot", "Quest log tag such as group or dungeon; null when absent or uncorroborated."},
		{"settled", "Everloot", "True when Everloot's settled list for the build holds the quest (five installs agree and no other value for 14 days)."},
		{"reporters", "Everloot", "Distinct installs that reported the quest."},
	}},
	{"vendor_sources", everloot.VendorSource{}, "Everloot observed vendor offers (CC BY-SA 4.0). Data from Everloot (https://everloot.app), CC BY-SA 4.0.", "One row per item a vendor sells, kept when the vendor is settled or at least two distinct installs listed the item for it, sorted by npc and item. The same offers are loot_sources rows of source_kind sold by.", []Col{
		{"npc_id", "Everloot", "Vendor creature id."},
		{"item_id", "Everloot", "Item id."},
		{"price_copper", "Everloot", "Price in copper as most installs saw it (the lowest on a tie)."},
		{"limited", "Everloot", "True when any install saw limited stock on the offer; null otherwise."},
		{"reporters", "Everloot", "Distinct installs that listed the offer."},
	}},
	{"item_sets", tables.ItemSetRow{}, wago + "ItemSet", "One row per member item of an item set (ItemID_0 to ItemID_16), sorted by set id and item id.", []Col{
		{"set_id", "", "Item set id."},
		{"name", "", "Item set name."},
		{"item_id", "", "items.id of a member of the set."},
	}},
	{"quest_lines", tables.QuestLineRow{}, wago + "QuestLine, QuestLineXQuest", "One row per quest of a quest line, in line order.", []Col{
		{"line_id", "", "Quest line id (quests.quest_line_id)."},
		{"name", "QuestLine", "Quest line name; null when QuestLine has no named row."},
		{"position", "QuestLineXQuest OrderIndex", "Position of the quest in the line, from 0."},
		{"quest_id", "", "quests.id of the quest at that position."},
	}},
	{"recipe_items", tables.RecipeItem{}, wago + "ItemXItemEffect, ItemEffect, SpellEffect, ItemSparse", "Items (Patterns, Plans, Recipes, Formulae, Schematics, Manuals) whose use spell has a LearnSpell effect that teaches a recipes spell. Where to get the item: join loot_sources on item_id (creature, object, quest reward, sold by).", []Col{
		{"item_id", "ItemXItemEffect ItemID", "The item that teaches the recipe."},
		{"teaches_spell_id", "SpellEffect EffectTriggerSpell", "recipes.spell_id the item teaches."},
		{"profession", "SkillLine", "Skill line name of the recipe."},
		{"required_skill", "ItemSparse RequiredSkillRank", "Skill rank the item itself needs; null when the client stores zero."},
	}},
	{"trainers", tables.Trainer{}, everlootTrainers, "One row per trainer and profession that at least two installs saw teaching a recipe, sorted by npc. Zone is Everloot's located_in zone for the creature; position and faction are null: a trainer window carries neither.", []Col{
		{"npc_id", "", "Trainer creature id."},
		{"name", "npcs", "Creature name when npcs holds the id; null otherwise."},
		{"profession", "", "Profession the trainer teaches."},
		{"zone_id", "Everloot located_in", "UiMap id of the zone; null when Everloot places the creature nowhere."},
		{"zone_name", "UiMap", "Name of that UiMap."},
		{"x", "none", "Always null: no allowed source gives a trainer position."},
		{"y", "none", "Always null."},
		{"faction", "none", "Always null: no allowed source gives it."},
		{"recipes", "derived", "Corroborated recipes the trainer offers (rows in trainer_spells)."},
		{"reporters", "", "Most installs that agreed on one of its offers."},
	}},
	{"trainer_spells", tables.TrainerSpell{}, everlootTrainers, "One row per recipe a trainer teaches that at least two distinct installs saw.", []Col{
		{"npc_id", "", "Trainer creature id."},
		{"spell_id", "recipes", "recipes.spell_id when exactly one recipe of that profession carries the offered name; null otherwise (the window names the recipe, not the spell)."},
		{"recipe_name", "", "Recipe name as the install saw it."},
		{"profession", "", "Profession of the offer."},
		{"cost_copper", "", "Training cost in copper; null unless two installs agree."},
		{"required_skill", "", "Skill rank the trainer requires; null unless two installs agree."},
		{"reporters", "", "Distinct installs that saw the offer."},
	}},
	{"gathering_nodes", tables.GatheringNode{}, "Everloot loot windows (CC BY-SA 4.0), wago.tools Lock, LockType, GameObjects. Data from Everloot (https://everloot.app), CC BY-SA 4.0.", "One row per herb or mining node in a zone that at least two installs looted. Empty until the loot windows are fetched (wowc-data fetch -loot).", []Col{
		{"node_id", "", "Game object id."},
		{"name", "GameObjects, Everloot", "Client name when the client knows the node, else Everloot's."},
		{"profession", "Lock, Everloot gatherKind", "Mining or Herbalism: from the client lock, else from Everloot's ore or herb label."},
		{"skill_required", "Lock", "Skill rank the client lock needs; null when the client does not hold the node."},
		{"zone_id", "Everloot mapId", "UiMap id of the zone the windows were taken in."},
		{"zone_name", "UiMap", "Name of that UiMap."},
		{"windows", "", "Loot windows seen at the node in the zone."},
		{"reporters", "", "Distinct installs."},
	}},
	{"gathering_yields", tables.GatheringYield{}, "Everloot loot windows (CC BY-SA 4.0). Data from Everloot (https://everloot.app), CC BY-SA 4.0.", "What a gathering node gave: items at least two installs saw from it in a loot window, plus items Everloot's corroborated drop rates give the node.", []Col{
		{"node_id", "", "gathering_nodes.node_id."},
		{"item_id", "", "Item gathered."},
		{"windows", "", "Windows holding the item."},
		{"reporters", "", "Distinct installs."},
		{"min_count", "", "Smallest stack seen; 0 on a yield taken from the drop rates, which hold no stack size."},
		{"max_count", "", "Largest stack seen; 0 on a yield taken from the drop rates."},
	}},
	{"fishing_catches", tables.FishingCatch{}, "Everloot loot windows (CC BY-SA 4.0). Data from Everloot (https://everloot.app), CC BY-SA 4.0.", "What fishing gave in a zone, kept when at least two installs caught the item there. Empty until the loot windows are fetched and carry fishing.", []Col{
		{"zone_id", "Everloot mapId", "UiMap id."},
		{"zone_name", "UiMap", "Name of that UiMap."},
		{"item_id", "", "Item caught."},
		{"casts", "", "Fishing windows seen in the zone."},
		{"catches", "", "Windows holding the item."},
		{"reporters", "", "Distinct installs that caught it."},
		{"rate", "derived", "catches divided by casts."},
	}},
	{"disenchant_yields", everloot.DisenchantYield{}, "Everloot /api/rollups/disenchant (CC BY-SA 4.0). Data from Everloot (https://everloot.app), CC BY-SA 4.0.", "What disenchanting gives, per band of item class, quality and item level, one row per material, exactly as Everloot rates it.", []Col{
		{"band_id", "", "Everloot's band id."},
		{"item_class", "", "armor or weapon."},
		{"quality", "", "Item quality name (uncommon, rare, epic)."},
		{"min_item_level", "", "Lowest item level of the band."},
		{"max_item_level", "", "Highest item level of the band."},
		{"state", "", "Everloot's state for the band: rated, short (few windows) or none."},
		{"windows", "", "Disenchants observed in the band."},
		{"installs", "", "Distinct installs."},
		{"material_item_id", "", "Material item id."},
		{"chance", "", "Share of disenchants that gave the material."},
		{"per_disenchant", "", "Average count per disenchant."},
		{"min_count", "", "Smallest count seen."},
		{"max_count", "", "Largest count seen."},
	}},
	{"profession_notes", tables.ProfessionNote{}, "curated YAML (curated/professions.yaml), advisor defaults", "Curated profession notes in our own words: which professions are primary or secondary, rank tiers with skill caps and the character level they need, pairings, feeders, special cases and rules of thumb.", []Col{
		{"section", "", "professions, ranks, pairings, feeders, special_cases or rules."},
		{"subject", "", "The profession, rank, first profession of a pairing, feeder, or rule topic."},
		{"related", "", "professions: primary or secondary; pairings and feeders: the other profession; null elsewhere."},
		{"skill_min", "", "ranks only: skill the rank starts at."},
		{"skill_cap", "", "ranks only: skill the rank ends at."},
		{"min_level", "", "ranks only: character level needed to learn the rank."},
		{"text", "", "The note."},
		{"source", "", "Always \"curated (advisor defaults)\"."},
	}},
	{"names", names.Row{}, "derived from items, npcs, instances, recipes and item_sets", "Prebuilt name index: one row per named item, npc, instance, recipe and item set, sorted by norm, with a Bloom filter on norm. Quests carry no name, so none are listed. An exact lookup reads the row groups whose norm min/max range and Bloom filter admit the name; a prefix lookup reads those whose min/max range overlaps the prefix (see layout.md).", []Col{
		{"norm", "derived", "The name lowercased, accent-folded, apostrophes dropped and other punctuation turned into single spaces."},
		{"name", "", "The name as the source table spells it."},
		{"table", "", "Table the name belongs to: items, npcs, instances, recipes or item_sets."},
		{"id", "", "Row id in that table: items.id, npcs.id, instances.map_id, recipes.spell_id, item_sets.set_id."},
	}},
}
