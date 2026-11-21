# Data sources

## wago.tools (client tables)

Status: **client tables retrieved via wago.tools may be used, within the limits below (Operator ruling, 2026-11-20).**

Credit line: "Blizzard game client data, retrieved via wago.tools". The underlying game data is Blizzard's.

Ruling and limits:

- The Forever client's own tables, retrieved via wago.tools (which mirrors them and states no terms of its own), may be used, and facts derived from them published.
- Publish facts only: ids, numbers, levels, flags, slots, names and short labels.
- Never publish long-form text: quest descriptions, progress and completion text, item flavour text, book pages or gossip.
- Extracting the tables from the installed client is the long-term route, so the dataset does not depend on a site without published terms.

Route and pipeline: `/db2/{table}/csv?version={build}`, one request per table, one at a time with a pause between, cached under `dist/cache/{build}/` (git-ignored) with a `provenance.json` (source, product, build, retrieved time, the ruling as the license basis). Tables fetched: `ItemSparse`, `Item`, `QuestV2`, `QuestLineXQuest`, `QuestPOIBlob`, `Creature`, `CreatureDifficulty` (the last two feed `npcs`: name, classification, creature type, family, faction template and level range; no title or gossip text, no spawn coordinates).

Product confirmation (2026-11-20): product `wowf` is the WoW: Forever client.

- wago.tools' build list (`/api/builds`) shows `wowf` at version `1.60.1.71037` (created 2026-11-20 18:22:06), with earlier builds `1.60.1.70959` and `1.60.1.70938`. Its version series is 1.60.1.x.
- Corroboration: Blizzard's own forum post "Beta Client Update - September 22" (https://us.forums.blizzard.com/en/wow/t/beta-client-update-september-22/2358655) describes WoW Forever beta as version 1.60.1, build 69977, and the Warcraft wiki's `Patch 1.60.1` page (https://warcraft.wiki.gg/wiki/Patch_1.60.1) redirects to its "World of Warcraft: Forever" article. The wago.tools product code `wowf` itself is not named in either source; the match rests on the shared 1.60.1 version series, with wago.tools' build numbers (70938 to 71037) following the beta build 69977.
- Build used: `1.60.1.71037`.

Checked on 2026-11-20:

- `https://wago.tools/terms`, `/privacy` and `/about` return 404. The site publishes no terms page.
- `https://wago.tools/robots.txt` disallows `/db2` browsing and allows only `/db2/{table}/csv` exports and `/api/`.
- `https://wago.tools/llms.txt` describes `/db2/{table}/csv` as the permitted route for table downloads, with exact-version selection (`?version=`). It states no license, no redistribution terms and no attribution requirement.
- `https://wago.tools/apis` states no license or terms either.
- `https://wago.tools/api/builds/latest` lists product `wowf` at version `1.60.1.71037`.

The site states no terms; the ruling above is what permits use.

## Everloot (loot sources)

Status: **usable. Everloot's data is CC BY-SA 4.0, credited, with a paginated JSON API that needs no key.** Checked 2026-11-20.

- Licence: the repository README (https://github.com/Everloot/everloot) says "Published data: CC BY-SA 4.0, credited to Everloot contributors" (code is AGPL-3.0). `docs/DATA_POLICY.md` in that repository says: "Anyone may read it, copy it and build on it under CC BY-SA 4.0. What you build from it stays under the same licence." The site footer (https://everloot.app/docs) says "Everloot data CC BY-SA 4.0".
- Attribution wording (`docs/DATA_POLICY.md`): "Data from Everloot (https://everloot.app), CC BY-SA 4.0." Republishing without credit breaks the licence. Every answer also carries an `attribution` object and `X-Everloot-Licence`, `X-Everloot-Credit` and `Link` headers.
- Route: `GET https://everloot.app/api/rollups/drops?offset=&limit=` (OpenAPI 3.1 at https://everloot.app/api/openapi.json; API reference https://everloot.app/docs/api). "Corroborated drop rates with loot-window counts." Lists answer with `total` and `next` (the next page's full URL, `null` on the last); no key is needed to read. Without a key the API allows 30 requests per hour per address and 500 records per page; a free key (made at /docs/api/keys after signing in with Battle.net) raises that to 2,000 requests per hour and 5,000 records per page. A 429 carries `Retry-After`. `docs/DATA_POLICY.md` also names a `corpus.ndjson` open-data file but gives no URL; the pipeline does not use it.
- Rules (`docs/DATA_POLICY.md`): "No crawlers, no copying another database's rows, guides or text." The machine-readable endpoints are the intended route; the pipeline makes one request per page, 2 s apart, and stops on any error.
- Update cadence: the corpus snapshot "rebuilt every 10 minutes" (repository README); every answer's `dataAsOf` says when the uploads behind it were current, and the pipeline records it as the table's version.
- Data model: a rate row carries `sourceNpcId` or `sourceObjectId`, `itemId`, `kills` (loot windows opened, not kills), `drops`, `rate` (drops / kills), `rateLow` and `rateHigh` (95% Wilson range), `reporters` (distinct installs), `public`, and `tier` (`reported`, `verified` or `confirmed`).

Pipeline: `wowc-data fetch` follows `next` through every page and caches the union as `dist/cache/everloot/drops.json` (git-ignored) with the retrieval time, `dataAsOf` and attribution. `loot_sources` keeps rows with `public: true` that name a creature or object source; values are stored as Everloot gives them. The table's Parquet metadata and manifest source carry the credit line and `dataAsOf`.

Quest rewards: neither the client tables fetched here (no quest reward items; QuestV2 has three columns, and no QuestPackageItem or QuestObjective table exists for this build) nor the drops route carry them. `loot_sources` takes `quest reward` rows from Everloot's `GET /api/edges?rel=reward_from` (item to quest, `installs` as `reporters`, `evidence` as `tier`; rate fields zero). Checked 2026-11-21: that relation held 0 rows. Not covered: vendor sources.

## Talent trees (client tables)

Checked 2026-11-21 against build `1.60.1.71037`, same route and ruling as above. This client generation uses the trait-tree family (a classic-style `Talent`/`TalentTab` pair also exists and is not used).

Tables fetched: `TraitNode`, `TraitNodeGroup`, `TraitNodeGroupXTraitNode`, `TraitNodeXTraitNodeEntry`, `TraitNodeEntry`, `TraitDefinition`, `TraitEdge`, `TraitCond`, `SkillLineXTraitTree`, `SkillRaceClassInfo`, `ChrClasses`, `SpellName`.

- `talent_trees`: one row per `TraitNodeGroup` (tree, group, order by group id). `required_points` is `TraitCond.SpentAmountRequired` for the group. The class comes from `SkillLineXTraitTree` (tree to skill line), `SkillRaceClassInfo` (skill line to class mask) and `ChrClasses`; 9 trees resolve to a class, the others (34 groups) have none.
- `talent_nodes`: one row per `TraitNode`. A node sits in many groups, so `group_ids` is a list. Spell, name and max rank come from the node's first entry by `_Index` (`TraitNodeXTraitNodeEntry`, `TraitNodeEntry`, `TraitDefinition`); the name is the definition's override name or, if empty, the spell's name from `SpellName`. `prerequisite_node_ids` are the `LeftTraitNodeID`s of `TraitEdge` rows pointing at the node, whatever the edge type.
- Names the tables do not carry: no table holds a tree or group name (`TraitTree.TitleText_lang` and `TraitNodeGroup` have no name), so `group_name` is null. The class's specialisation names exist only as skill lines (priest: "Holy", "Shadow Magic", "Discipline") with no key tying them to a group.

## Curated talent builds

`curated/talents/` is hand-written for this project (no guide-site text) and published as `talent_builds`, with the source "curated (advisor defaults)". Node ids, names and max ranks are checked against the talent tables above at every `make dataset`. See `curated/README.md`.

## Quest level, minimum level and zone (survey)

Surveyed 2026-11-21 against build `1.60.1.71037` (wago.tools) and Everloot's API. Row counts are data rows.

| Source | Rows | Holds |
| ------ | ---: | ----- |
| `QuestV2` | 6,626 | `ID`, `UniqueBitFlag`, `UiQuestDetailsThemeID` only: no level, minimum level or zone |
| `QuestPOIBlob` | 57 | `QuestID` to `MapID` and `UiMapID` (22 quests with a non-zero `UiMapID`) |
| `UiMap` | 60 | `ID` to `Name_lang` (zone, continent and city names) |
| `QuestLine`, `QuestLineXQuest` | 3 and 22 | quest line membership; no level or zone |
| `QuestSort` | 39 | sort names ("Epic", old dungeon labels), no key from `QuestV2` |
| `QuestInfo` | 7 | quest type names, no key from `QuestV2` |
| `QuestXP` | 100 | XP by level and difficulty; keyed by level, not by quest |
| `ContentTuning` | 102 | level squish, scaling and `XpMultQuest` by tuning id; nothing in the quest tables points at a tuning id |
| `AreaTable` | 1,404 | area names; nothing ties a quest to an area |
| `QuestPOIPoint` | 121 | coordinates for POI blobs; no level or zone |
| `QuestV2CliTask`, `QuestObjective`, `Quest` | – | "Table not found" |
| Everloot `/api/edges?rel=located_in` | 540 | npc to zone only; no quest rows. `reward_from` holds 0 rows; the client layer (`/api/edges/client-layer`) has no quest relations. |

Outcome, per `quests` column:

- `level`, `min_level`: **no allowed source holds them**, so both are always null. Nothing is inferred (not from `QuestXP`, `ContentTuning` or the quest's zone).
- `zone_id`: the `UiMapID` of the quest's first `QuestPOIBlob` row (lowest `ID`), when non-zero. Source: wago.tools `QuestPOIBlob`.
- `zone_name`: `UiMap.Name_lang` for that `zone_id`; null when `UiMap` has no named row. Source: wago.tools `UiMap`. It is a short map label.

`UiMap` is fetched with the other client tables and its `ID` and `Name_lang` columns are required (a missing column is an error, as for every table).

Live coverage (2026-11-21 run): see the mission report; zone is non-null for 22 of 6,626 quests (0.33%), level and min_level for none.

## Item stat values, armor, damage and speed (survey)

Surveyed 2026-11-21 against build `1.60.1.71037` (wago.tools). Row counts are data rows.

| Source | Rows | Holds |
| ------ | ---: | ----- |
| `ItemSparse` | 19,400 | `StatModifier_bonusStat_0..9` (stat type ids, -1 for an empty slot), `StatPercentEditor_0..9` (a share of a level budget in 1/10000, e.g. 10000, 8000; not an amount), `ItemDelay` (weapon speed in ms; non-zero on 1,926 rows), `ItemLevel`, `DmgVariance`. No stat amount, armor or damage column. |
| `Item` | 32,009 | `ClassID`, `SubclassID`, `InventoryType`; no stats |
| `RandPropPoints` | 300 | per-item-level budget tables (`Epic_*`, `Superior_*`, `Good_*`) |
| `ItemArmorTotal` | 100 | armor by item level for cloth, leather, mail, plate |
| `ItemArmorQuality`, `ItemArmorShield` | 100, 100 | armor quality multipliers; shield armor by item level |
| `ItemDamageOneHand`, `ItemDamageTwoHand` (and `...Caster`), `ItemDamageAmmo` | 100 (ammo 1,000) | damage per second by item level and quality |
| `ItemStatEffect`, `ItemSparseStat`, `ItemStat`, `ItemScalingTable` | – | "Table not found" |

Finding: this build stores no per-item stat amount, armor value or damage range. The amounts are computed by the client from the budget tables above, the item's level, quality and slot, and `StatPercentEditor`. Reproducing that is a derivation, not a read of client data, and the rule is that no value is inferred. So the budget tables are not fetched and:

- `items.stats`: one `{type, value}` per listed stat, in slot order. `type` is `StatModifier_bonusStat_n` (ItemSparse). `value` is always null.
- `items.armor`, `dmg_min`, `dmg_max`: always null.
- `items.speed_ms`: `ItemSparse.ItemDelay` for weapon-class items (`Item.ClassID` 2) with a non-zero delay; null otherwise. This is a direct client value.
- `items.stat_types` (existing, ids only) is kept unchanged.
- `stat_types` table (id, label): the client's item stat enum (ITEM_MOD_*) ids 0 to 56 with short labels, written as constants in `internal/tables/itemstats.go`; it is not fetched from a table (the build has no table for it). The enum is from the public documentation of that enum, not verified against this client. Stat ids above 56 appear on items (83 to 136, 1 to 87 items each) and have no label.

`ItemSparse` columns `StatPercentEditor_0` and `ItemDelay` are required (a missing column is an error, as for every table).

Live coverage (2026-11-21 run, required level 1 to 30): among armor (`ClassID` 4) and weapon (`ClassID` 2) items, 711 of 2,588 (27.5%) list at least one stat type (armor 591 of 1,663, 35.5%; weapons 120 of 925, 13.0%). With a stat amount (`stats.value`): 0 of 2,588. With armor, `dmg_min` or `dmg_max`: 0. With `speed_ms`: 908 of 925 weapons.

## Profession recipes, reagents and skill-up ranges (survey)

Surveyed 2026-11-21 against build `1.60.1.71037` (wago.tools). Row counts are data rows.

| Source | Rows | Holds |
| ------ | ---: | ----- |
| `SkillLine` | 154 | `ID`, `DisplayName_lang`, `CategoryID` (11 = primary profession, 9 = secondary profession and also riding and racial lines), `ParentSkillLineID`. Two "Test Profession [DNT]" lines (2933, 2934) are excluded. |
| `SkillLineAbility` | 7,853 | `SkillLine` to `Spell`, with `MinSkillLineRank`, `TrivialSkillLineRankLow`, `TrivialSkillLineRankHigh`, `AcquireMethod`, `NumSkillUps` |
| `SpellReagents` | 3,319 | one row per spell: `Reagent_0..7` (item ids), `ReagentCount_0..7` |
| `SpellEffect` | 43,538 | `SpellID`, `EffectIndex`, `Effect` (24 CreateItem on 3,351 rows, 157 CreateRandomItem on 18), `EffectItemType` (the item made) |
| `SpellName` | 32,503 | spell names (already fetched for talents) |

Tables fetched for this: `SkillLine`, `SkillLineAbility`, `SpellReagents`, `SpellEffect`. Their columns used are required (a missing column is an error, as for every table).

- `recipes`: one row per `SkillLineAbility` row whose `SkillLine` is a profession line (`CategoryID` 11 or 9, no `[DNT]` line) and whose spell has at least one reagent or a CreateItem/CreateRandomItem effect, once per (skill line, spell). That filter leaves out racial and riding abilities, gathering abilities with no reagent or item (Find Minerals and the like), and recipes with neither. Columns: `spell_id` (`Spell`), `skill_line_id`, `skill_line_name` (`SkillLine.DisplayName_lang`), `name` (`SpellName.Name_lang`, null when empty), `creates_item_id` (`SpellEffect.EffectItemType` of the lowest `EffectIndex` item-making effect; null when none, e.g. enchants), `learn_skill` (`MinSkillLineRank`), `yellow_skill` (`TrivialSkillLineRankLow`), `grey_skill` (`TrivialSkillLineRankHigh`). A 0 in a rank column means none and is stored as null.
- Which rank is which colour follows the commonly documented reading of these columns (low = yellow, high = grey); the client's own files do not label them. The client stores no green threshold, so `recipes` has no green column; nothing is derived from the other two.
- `recipe_reagents`: one row per reagent of each `recipes` spell, from `SpellReagents` slots 0 to 7 where the item id and the count are both positive; a reagent listed in two slots is summed. `ReagentReCraftCount_n` and `ReagentSource_n` are not read.
- No description text is read.

Live coverage (2026-11-21 run): 2,522 recipes and 8,475 reagent rows. Recipes per skill line: Blacksmithing 506, Leatherworking 592, Tailoring 471, Engineering 275, Enchanting 262, Alchemy 196, Cooking 133, First Aid 32, Poisons 26, Mining 18, Herbalism 3, Fishing 3, Skinning 3, Survival 1, Comprehension 1. All have reagents; 2,337 create an item (Enchanting: 82 of 262); 2,520 carry both yellow and grey ranks.

## Dungeons, raids and their bosses (survey)

Surveyed 2026-11-21 against build `1.60.1.71037` (wago.tools). Row counts are data rows.

| Source | Rows | Holds |
| ------ | ---: | ----- |
| `Map` | 78 | `ID`, `MapName_lang`, `InstanceType` (0 open world, 1 dungeon, 2 raid, 3 battleground), `CorpseMapID` (the map of the entrance, -1 when none), `Corpse_0` and `Corpse_1` (entrance position, 0 when none), `MaxPlayers`, `AreaTableID` (0 on most instance maps) |
| `DungeonEncounter` | 376 | `ID`, `Name_lang`, `MapID`, `DifficultyID` (0 on nearly all), `OrderIndex`; no creature id |
| `MapDifficulty` | 142 | `MapID` to `ContentTuningID` |
| `ContentTuning` | 102 | `LfgMinLevel`, `LfgMaxLevel` (non-zero on 2 rows; the other level columns are squish and scaling values, not a level range) |
| `LFGDungeons` | 76 | names and `ContentTuningID`; `MapID` is 0 on every row, so it cannot be tied to a map except by name |
| `AreaTable` | 1,404 | area names; no key from an instance map to its entrance zone |
| `JournalInstance`, `JournalEncounter`, `JournalEncounterCreature`, `JournalEncounterItem` | – | "Table not found": no encounter-to-creature or encounter-to-loot link in the client tables |
| Everloot `/api/edges` | – | `located_in` is npc to zone; nothing ties an npc to an instance or encounter |

Tables fetched for this: `Map`, `DungeonEncounter`, `MapDifficulty`, `ContentTuning`. Their columns used are required (a missing column is an error, as for every table). `Creature` (already fetched) is read for the boss link.

- `instances`: one row per `Map` with `InstanceType` 1 or 2, leaving out placeholder maps (empty name, a name starting with `<`, or containing "Test"). Columns: `map_id`, `name` (`MapName_lang`), `instance_type` (`dungeon` or `raid`, as the client types it; the client also types a few city and scenario maps as dungeons), `max_players` (`MaxPlayers`, null when 0), `entrance_map_id` (`CorpseMapID`, null when -1), `entrance_x` and `entrance_y` (`Corpse_0`, `Corpse_1`, null when both 0), `min_level` and `max_level` (`ContentTuning.LfgMinLevel` and `LfgMaxLevel` of the map's lowest-id `MapDifficulty` row that has a non-zero pair; null otherwise).
- Zone of the entrance: no table names it (`AreaTable` holds no map-to-entrance key, and the entrance position is a world coordinate), so there is no zone column; `entrance_map_id` and position are what the client gives. Levels are never inferred from `LFGDungeons`, creature levels or the content tuning squish values.
- `instance_bosses`: one row per `DungeonEncounter` whose `MapID` is an `instances` map. Columns: `map_id`, `encounter_id` (`ID`), `name`, `order_index`, `difficulty_id`, and `creature_id`. The client has no encounter-to-creature key, so `creature_id` is the one derived column: the `Creature.ID` when exactly one creature carries the encounter's exact name, null otherwise. Join `creature_id` to `loot_sources` (`source_kind` creature) for drops.
- No description text is read.

Live coverage (2026-11-21 run): 52 instances (39 dungeon, 13 raid) and 372 boss rows on 44 of them. Entrance map on 27 instances (position on 25), max players on 31, min and max level on 1. `creature_id` on 0 of 372 bosses: the fetched `Creature` table holds only 189 rows and none matches an encounter name, so no boss links to `loot_sources` (52 creature sources there).

## Crafted sources in `loot_sources`

`loot_sources` also holds `source_kind` `crafted` rows, derived only from the client `recipes` table (no Everloot data, no network beyond the recipe fetch). One row per recipe whose `creates_item_id` is set (a spell listed under two skill lines gives one row, the lower skill line id):

- `item_id`: the recipe's `creates_item_id`; `source_id`: the recipe spell id (`recipes.spell_id`).
- `profession`: the recipe's skill line name (`skill_line_name`); `learn_skill`: its `learn_skill` (minimum skill rank to learn it), null when the client stores zero.
- `kills`, `drops`, `rate` and `reporters` are 0 and `rate_low`, `rate_high`, `tier` and `everloot_data_as_of` are null: there is no drop chance.

`profession` and `learn_skill` are new nullable columns, null on every creature, object and quest reward row; those rows are otherwise unchanged. The table's Parquet metadata and manifest sources list the Everloot credit and the client source. `item_id` is not checked against `items`: a created item the fetched item tables lack still gets a row (329 live).

Live coverage (2026-11-21 run, build 1.60.1.71037): 2,336 crafted rows of 2,667 in `loot_sources`. By profession: Leatherworking 592, Blacksmithing 506, Tailoring 470, Engineering 272, Alchemy 196, Cooking 132, Enchanting 82, First Aid 32, Poisons 26, Mining 18, Fishing 3, Herbalism 3, Skinning 3, Comprehension 1. Existing rows: 294 creature, 37 object, 0 quest reward.

## Creature zones (survey)

Surveyed 2026-11-21 against build `1.60.1.71037` (wago.tools) and Everloot's API.

| Source | Rows | Holds |
| ------ | ---: | ----- |
| Everloot `/api/edges?rel=located_in` | 542 | creature (`npc`) to zone, 531 distinct creatures, 33 distinct zones, `evidence` `observed` on every row, `installs` per edge. The zone id is a client `UiMap` id (all 33 are named in `UiMap`: 1413 The Barrens, 1434 Stranglethorn Vale, 1458 Undercity). |
| wago.tools `CreatureSpawn`, `CreatureXMap`, `SpawnGroup`, `CreatureXContribution` | – | "Table not found": the client tables carry no creature spawn or placement data |
| `Creature`, `CreatureDifficulty` | 189, – | no map, area or zone column |
| `UiMap` | 60 | `ID` to `Name_lang` (already fetched for quests) |
| `instance_bosses.creature_id` (DATA-014) | 0 of 372 | the only instance link for a creature is null on every boss, so no creature gets a zone from its instance |

Outcome, per `npcs` column:

- `zone_id`: the `UiMap` id of the creature's `located_in` edge. A creature with several edges takes the one with the most `installs`, the lowest zone id on a tie. Source: Everloot (CC BY-SA 4.0, credited as for `loot_sources`).
- `zone_name`: `UiMap.Name_lang` for that id; null when `UiMap` has no named row.
- `zone_source`: `everloot located_in` on every row that has a zone; null otherwise. Nothing is inferred (not from the creature's level, faction or name, nor from the drop table).
- No client table or instance link supplies a zone, so Everloot is the only source. The edges are fetched with the drop rates and cached in `dist/cache/everloot/drops.json`; a cache written before this column existed holds no edges and has to be deleted to refetch.

`loot_sources` has no zone column: a creature source's zone is its creature's, so join `loot_sources.source_id` to `npcs.id` where `source_kind` is `creature`, and read `zone_id`, `zone_name` and `zone_source`. A denormalized column would hold the same values and would not reach more rows.

Live coverage (2026-11-21 run): `npcs` has 189 rows and 0 have a zone. Everloot places 531 creatures but none of them is among the 189 client creature rows, and none is among the 52 creatures that `loot_sources` names (294 creature rows). So 0 of 294 creature sources (0.0%) have a zone, by the join or by id. The two Everloot sets (creatures with a drop and creatures with a location) do not overlap, and the client `Creature` table fetched for this build is a small slice of the creatures that exist.

## Item sell and buy prices, item sets and quest lines (survey)

Surveyed 2026-11-21 against build `1.60.1.71037` (wago.tools). Row counts are data rows.

| Source | Rows | Holds |
| ------ | ---: | ----- |
| `ItemSparse` | 19,400 | `SellPrice` and `BuyPrice` in copper (non-zero on 12,040 and 12,679 rows); also `PriceVariance`, `PriceRandomValue`, not used |
| `ItemSet` | 545 | `ID`, `Name_lang`, `ItemID_0..16` (0 for an empty slot); 541 sets list at least one item |
| `ItemSetSpell` | 1,505 | set bonus spells by threshold; not used (spell text, no item facts) |
| `QuestLine` | 3 | `ID`, `Name_lang` (`Description_lang` is long-form text and is not read) |
| `QuestLineXQuest` | 22 | `QuestLineID`, `QuestID`, `OrderIndex` (the position in the line) |

Outcome:

- `items.sell_price`, `items.buy_price`: copper from `ItemSparse`; null when the client stores zero (no price). A zero is not read as "sells for nothing".
- `item_sets` (`set_id`, `name`, `item_id`): one row per set and member item from `ItemSet.ItemID_0..16`, empty slots dropped, an item repeated in a set kept once. 3,141 rows over 541 sets. Only 895 of the 3,141 member ids are in `items`: `ItemSet` lists ids for pieces this client's `ItemSparse` does not carry.
- `quest_lines` (`line_id`, `name`, `position`, `quest_id`): 22 rows over 3 lines, ordered by `OrderIndex` from 0. This is every quest line this build has. `quests.quest_line_id` (the first line of a quest) is unchanged.

The tables `ItemSet` and `QuestLine` are fetched with the others, and the columns above are required (a missing column is an error, as for every table).

Live coverage (2026-11-21 run): `sell_price` non-null on 12,040 of 19,400 items (62.1%), `buy_price` on 12,679 (65.4%).
