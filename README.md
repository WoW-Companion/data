# WoW Companion data

This repository builds the public WoW: Forever reference dataset (items, quests, NPCs, loot sources and more) as versioned Parquet files with a manifest recording every table's sources, game build, hashes and licenses. It also holds the curated YAML that people review in pull requests.

> WoW Companion is a fan project. It is not affiliated with or endorsed by Blizzard Entertainment. World of Warcraft and Warcraft are trademarks of Blizzard Entertainment, Inc.

## Planned contents

- Parquet tables (zstd), one per entity per game build, sorted by primary lookup key: items, quests, NPCs, loot sources, and precomputed joins for advice.
- A manifest per dataset version with row counts, sha256 hashes, sort keys and per-source provenance.
- `curated/`: hand-maintained YAML (stat weights, talent builds, profession routes).

Every table and column is documented in [`docs/schema.md`](docs/schema.md), and `wowc-data coverage` reports how much of each column is filled (see `docs/releases.md`).

## Data sources and credit

Loot sources come from Everloot (https://everloot.app), CC BY-SA 4.0. Game data comes from the WoW: Forever client's own tables. Credit: Blizzard game client data, retrieved via wago.tools. The underlying game data is Blizzard's. Only facts (ids, numbers, flags, slots, names) are published, never long-form text. See `docs/sources.md`.

No game data is committed to the repository; `dist/` is git-ignored.

## Layout

- `cmd/wowc-data/`: the CLI (`fetch`, `build`, `verify-manifest`, `schema`, `coverage`)
- `internal/`: pipeline code (manifest model and schema check, provenance stamping, client table fetch and build, Parquet writer)
- `spikes/`: re-runnable measurements (Parquet layout, Go benchmarks)
- `schemas/`: JSON Schemas for the published files
- `curated/`: hand-maintained YAML
- `dist/`: build output (git-ignored)
- `docs/`: documentation; the table and column reference is `docs/schema.md` (generated), dataset releases are described in `docs/releases.md`

## Development

Requires Go (CGO disabled; the Makefile sets it).

```
make verify    # gofmt check, go vet, go test; offline
make fmt       # gofmt -w
make fetch     # download client tables and Everloot drop rates once into dist/cache/
make dataset   # build dist/<build>/ (Parquet tables and manifest.json) and verify the manifest
go run ./cmd/wowc-data schema                # regenerate docs/schema.md (a test fails when it is stale)
go run ./cmd/wowc-data coverage dist/<build>  # null and zero share per column, as Markdown
make spike     # re-measure the Parquet layout (about a minute)
```

## Licensing

- Code is MIT, in `LICENSE`.
- The dataset is CC BY-SA 4.0, in `LICENSE-DATA`, because it includes Everloot's open data, which is share-alike.

The `loot_sources` table is Everloot's: "Data from Everloot (https://everloot.app), CC BY-SA 4.0." (see `docs/sources.md`).

Never redistributed here:

- Wowhead content (text, tooltips, comments, coordinates, drop rates) is not included in the dataset.
- forever-db's GPL tables are not included in the dataset.
