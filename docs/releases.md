# Dataset releases

## What is published

Each WoW: Forever game build gets one GitHub release, tagged `dataset-<build>` (for example `dataset-1.60.1.71037`). Assets:

- `wowc-dataset-<build>.tar.zst`: the contents of `dist/<build>/` (the Parquet tables and `manifest.json`) in a directory named `<build>`.
- `manifest-<build>.json`: the same `manifest.json` as a separate download, so a consumer can read hashes without fetching the archive.
- `coverage-<build>.md`: row counts and the null or zero share of every column (`wowc-data coverage`), so a consumer can see what is filled before downloading.
- `stats-<build>.json`: the same coverage with each column's type (`wowc-data coverage -json`), which the next release's notes are diffed against.

The release notes start with what changed since the previous dataset release, from `wowc-data diff <old dir> <new dir>`: tables added and removed, columns added, removed or type-changed, row counts old and new, null or zero shares that moved more than 5 points, and the dataset version and build on each side. The old directory needs only `manifest.json` and `stats.json` (the previous release's `manifest-<build>.json` and `stats-<build>.json`, renamed); the new one is a built dataset. A previous release that has no stats file is diffed on tables and rows only. The first release says "First release". The notes also carry the licence and credits: the dataset is CC BY-SA 4.0; "Blizzard game client data, retrieved via wago.tools"; "Data from Everloot (https://everloot.app), CC BY-SA 4.0."

## When

The `dataset` workflow runs daily (06:17 UTC) and on manual dispatch. It asks wago.tools for the latest `wowf` build (`wowc-data release-check`); if `dataset-<build>` already exists it stops, otherwise it fetches, builds, runs `verify-manifest`, and publishes (`scripts/pack-and-publish.sh`: archive, stats, notes from `wowc-data release-notes`, `gh release create`). A release is never rewritten.

Everloot allows 30 requests an hour per address without a key and answers 429 with `Retry-After` (about 23 minutes). A runner's address is shared, so `fetch` waits out a 429 (one wait of up to 30 minutes, 40 minutes in all); wago.tools downloads retry the same way on 429 and 5xx, up to 20 minutes. The job's timeout is 120 minutes.

`make release-dryrun` runs the whole job on a machine, in the same order, without publishing; it needs the network and `zstd`.

## Verifying a download

```
mkdir data && tar --zstd -xf wowc-dataset-<build>.tar.zst -C data
go run github.com/WoW-Companion/data/cmd/wowc-data@latest verify-manifest data/<build>
```

`verify-manifest` checks `manifest.json` against its schema and recomputes each listed file's SHA-256. Without Go, compare by hand: every entry in `manifest.json`'s `tables` has a `path` and a `sha256`, so `shasum -a 256 data/<build>/<path>` must print that hash. Also check that `manifest-<build>.json` is identical to `data/<build>/manifest.json`.
