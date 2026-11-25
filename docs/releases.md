# Dataset releases

## What is published

Each WoW: Forever game build gets one GitHub release, tagged `dataset-<build>` (for example `dataset-1.60.1.71037`). Assets:

- `wowc-dataset-<build>.tar.zst`: the contents of `dist/<build>/` (the Parquet tables and `manifest.json`) in a directory named `<build>`.
- `manifest-<build>.json`: the same `manifest.json` as a separate download, so a consumer can read hashes without fetching the archive.
- `coverage-<build>.md`: row counts and the null or zero share of every column (`wowc-data coverage`), so a consumer can see what is filled before downloading.
- `stats-<build>.json`: the same coverage with each column's type (`wowc-data coverage -json`), which the next release's notes are diffed against.

The release notes start with what changed since the previous dataset release, from `wowc-data diff <old dir> <new dir>`: tables added and removed, columns added, removed or type-changed, row counts old and new, null or zero shares that moved more than 5 points, and the dataset version and build on each side. The old directory needs only `manifest.json` and `stats.json` (the previous release's `manifest-<build>.json` and `stats-<build>.json`, renamed); the new one is a built dataset. A previous release that has no stats file is diffed on tables and rows only. The first release says "First release". The notes also carry the licence and credits: the dataset is CC BY-SA 4.0; "Blizzard game client data, retrieved via wago.tools"; "Data from Everloot (https://everloot.app), CC BY-SA 4.0."

## Revisions

`manifest.json` carries a `revision`: the first 12 hex digits of a SHA-256 over the inputs we control, which are every file under `curated/`, `internal/`, `cmd/` and `schemas/` (Go `_test.go` files excluded), by path and content. It is stable for an unchanged tree and changes when a curated build, the table-building code or a schema changes. Releases published before revisions existed have no `revision`, and count as different. The release notes carry a dated `Revision: <hash> (<date>)` line.

## When

The `dataset` workflow runs daily (06:17 UTC), on manual dispatch, and on every push to `main` that touches `curated/**`, `internal/**`, `cmd/**` or `schemas/**`. One run at a time (concurrency group `dataset`). It asks wago.tools for the latest `wowf` build and reads the revision published in `manifest-<build>.json` (`wowc-data release-check` prints `build`, `exists`, `revision`, `published`, `revision_differs` and `publish`). It stops when `dataset-<build>` exists with the current revision. Otherwise it fetches, builds, runs `verify-manifest`, and publishes (`scripts/pack-and-publish.sh`: archive, stats, notes from `wowc-data release-notes`).

- **No release for the build:** `gh release create`.
- **Release exists, revision differs:** the same release is republished. The new assets are uploaded as `new-<name>`, then each is swapped in by renaming (the old one is renamed `old-<name>`, the new one takes the real name, the old one is deleted), and the notes are rewritten. The release keeps its tag and always holds a manifest and archive pair.

Everloot allows 30 requests an hour per address without a key and answers 429 with `Retry-After` (about 23 minutes). A runner's address is shared, so `fetch` waits out a 429 (one wait of up to 30 minutes, 40 minutes in all); wago.tools downloads retry the same way on 429 and 5xx, up to 20 minutes. The job's timeout is 120 minutes.

`make release-dryrun` runs the whole job on a machine, in the same order, without publishing; it needs the network and `zstd`.

## Verifying a download

```
mkdir data && tar --zstd -xf wowc-dataset-<build>.tar.zst -C data
go run github.com/WoW-Companion/data/cmd/wowc-data@latest verify-manifest data/<build>
```

`verify-manifest` checks `manifest.json` against its schema and recomputes each listed file's SHA-256. Without Go, compare by hand: every entry in `manifest.json`'s `tables` has a `path` and a `sha256`, so `shasum -a 256 data/<build>/<path>` must print that hash. Also check that `manifest-<build>.json` is identical to `data/<build>/manifest.json`.

## Carry-forward and the shrink guard

`wowc-data build` (with `GITHUB_REPOSITORY` set) never publishes a regression:

- **Carry forward.** When an Everloot source (the corpus, or the disenchant rollup) could not be fetched, the tables built from it (`quests_observed`, `vendor_sources`, `trainers`, `trainer_spells`, `gathering_nodes`, `gathering_yields`, `fishing_catches`; `disenchant_yields`) take their rows from the newest published dataset release that has them: this build's own release first, then the others newest first. The release's manifest must show the table with rows and its Everloot source as fetched or already carried, never `unavailable`; the table file's sha256 is checked against that manifest. The manifest source records `version: carried-forward`, with the revision and `retrieved` time of the fetch it came from. With no such release the table builds empty.
- **Shrink guard.** After the build, each table's rows are compared with the newest published release. A table that had rows and drops by more than 50%, or to 0, fails the build with a line naming the table and both counts, so nothing is published. A deliberate shrink is listed in `curated/allow-shrink.yaml` (`allow_shrink:` entries with a `table` and a `reason`).
- **English names.** Everloot creature and node names with accented Latin letters or a non-Latin script are dropped (the client's English name wins); the build log counts them.

## Revision history

Every publish also keeps `wowc-dataset-<build>-<revision>.tar.zst` and `manifest-<build>-<revision>.json` on the build's release (the unsuffixed names the advisor downloads are unchanged), the last 10 revisions per build; older ones are deleted. `revisions-<build>.json` lists them newest first: revision, created, rows per table, and for Everloot-derived tables whether the source was `fetched`, `carried` or `unavailable`.

Carry-forward takes a table from the newest kept revision whose source was fetched and had rows, then from the latest releases as before. The shrink guard compares each table with the most rows the newest publish or any kept revision had. With no history yet (the first publish after this change) both behave as before; a republish adds the new revision, and the one it replaced is not back-filled.
