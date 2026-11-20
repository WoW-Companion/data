# Data sources

## wago.tools (client tables)

Status: **redistribution of derived data is not confirmed. No data has been fetched.**

Checked on 2026-11-20:

- `https://wago.tools/terms`, `/privacy` and `/about` return 404. The site publishes no terms page.
- `https://wago.tools/robots.txt` disallows `/db2` browsing and allows only `/db2/{table}/csv` exports and `/api/`.
- `https://wago.tools/llms.txt` describes `/db2/{table}/csv` as the permitted route for table downloads, with exact-version selection (`?version=`). It states no license, no redistribution terms and no attribution requirement.
- `https://wago.tools/apis` states no license or terms either.
- `https://wago.tools/api/builds/latest` lists product `wowf` at version `1.60.1.71037`, which may be the Forever client. This is unconfirmed.

The route may be used for download. Nothing found says what may be done with derived facts, so the pipeline stage and the measurements wait on an Operator decision.
