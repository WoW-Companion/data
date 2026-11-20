# WoW Companion data

This repository builds the public WoW: Forever reference dataset (items, quests, NPCs, loot sources and more) as versioned Parquet files with a manifest recording every table's sources, game build, hashes and licenses. It also holds the curated YAML that people review in pull requests.

> WoW Companion is a fan project. It is not affiliated with or endorsed by Blizzard Entertainment. World of Warcraft and Warcraft are trademarks of Blizzard Entertainment, Inc.

## Planned contents

- Parquet tables (zstd), one per entity per game build, sorted by primary lookup key: items, quests, NPCs, loot sources, and precomputed joins for advice.
- A manifest per dataset version with row counts, sha256 hashes, sort keys and per-source provenance.
- `curated/`: hand-maintained YAML (stat weights, talent builds, profession routes).

## Data sources and credit

Game data comes from the WoW: Forever client's own tables. Credit: Blizzard game client data, retrieved via wago.tools. The underlying game data is Blizzard's. Only facts (ids, numbers, flags, slots, names) are published, never long-form text. See `docs/sources.md`.

No game data is committed to the repository; `dist/` is git-ignored.

## Layout

- `src/wowc_data/`: pipeline code (manifest model and validation, provenance stamping, client table fetch and build)
- `spikes/`: re-runnable measurements (Parquet layout)
- `schemas/`: JSON Schemas for the published files
- `curated/`: hand-maintained YAML
- `tests/`: tests
- `dist/`: build output (git-ignored)
- `docs/`: documentation

## Development

Requires [uv](https://docs.astral.sh/uv/).

```
make verify   # ruff check, ruff format --check, pytest
make fmt      # apply lint fixes and formatting
uv run python -m wowc_data.build   # fetch client tables once, build dist/tables/
```

## Licensing

- Code is MIT, in `LICENSE`.
- The dataset is CC BY-SA 4.0, in `LICENSE-DATA`, because it includes Everloot's open data, which is share-alike.

Never redistributed here:

- Wowhead content (text, tooltips, comments, coordinates, drop rates) is not included in the dataset.
- forever-db's GPL tables are not included in the dataset.
