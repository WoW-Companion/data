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

Quest rewards: neither the client tables fetched here (no quest reward items; QuestV2 has three columns, and no QuestPackageItem or QuestObjective table exists for this build) nor the drops route carry them. `loot_sources` takes `quest reward` rows from Everloot's `GET /api/edges?rel=reward_from` (item to quest, `installs` as `reporters`, `evidence` as `tier`; rate fields zero). Checked 2026-11-21: that relation held 0 rows. Not covered: vendor and crafted sources.

## Talent trees (client tables)

Checked 2026-11-21 against build `1.60.1.71037`, same route and ruling as above. This client generation uses the trait-tree family (a classic-style `Talent`/`TalentTab` pair also exists and is not used).

Tables fetched: `TraitNode`, `TraitNodeGroup`, `TraitNodeGroupXTraitNode`, `TraitNodeXTraitNodeEntry`, `TraitNodeEntry`, `TraitDefinition`, `TraitEdge`, `TraitCond`, `SkillLineXTraitTree`, `SkillRaceClassInfo`, `ChrClasses`, `SpellName`.

- `talent_trees`: one row per `TraitNodeGroup` (tree, group, order by group id). `required_points` is `TraitCond.SpentAmountRequired` for the group. The class comes from `SkillLineXTraitTree` (tree to skill line), `SkillRaceClassInfo` (skill line to class mask) and `ChrClasses`; 9 trees resolve to a class, the others (34 groups) have none.
- `talent_nodes`: one row per `TraitNode`. A node sits in many groups, so `group_ids` is a list. Spell, name and max rank come from the node's first entry by `_Index` (`TraitNodeXTraitNodeEntry`, `TraitNodeEntry`, `TraitDefinition`); the name is the definition's override name or, if empty, the spell's name from `SpellName`. `prerequisite_node_ids` are the `LeftTraitNodeID`s of `TraitEdge` rows pointing at the node, whatever the edge type.
- Names the tables do not carry: no table holds a tree or group name (`TraitTree.TitleText_lang` and `TraitNodeGroup` have no name), so `group_name` is null. The class's specialisation names exist only as skill lines (priest: "Holy", "Shadow Magic", "Discipline") with no key tying them to a group.

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
