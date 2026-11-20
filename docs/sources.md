# Data sources

## wago.tools (client tables)

Status: **client tables retrieved via wago.tools may be used, within the limits below (Operator ruling, 2026-11-20).**

Credit line: "Blizzard game client data, retrieved via wago.tools". The underlying game data is Blizzard's.

Ruling and limits:

- The Forever client's own tables, retrieved via wago.tools (which mirrors them and states no terms of its own), may be used, and facts derived from them published.
- Publish facts only: ids, numbers, levels, flags, slots, names and short labels.
- Never publish long-form text: quest descriptions, progress and completion text, item flavour text, book pages or gossip.
- Extracting the tables from the installed client is the long-term route, so the dataset does not depend on a site without published terms.

Route and pipeline: `/db2/{table}/csv?version={build}`, one request per table, one at a time with a pause between, cached under `dist/cache/{build}/` (git-ignored) with a `provenance.json` (source, product, build, retrieved time, the ruling as the license basis). Tables fetched: `ItemSparse`, `Item`, `QuestV2`, `QuestLineXQuest`, `QuestPOIBlob`.

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
