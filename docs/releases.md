# Dataset releases

## What is published

Each WoW: Forever game build gets one GitHub release, tagged `dataset-<build>` (for example `dataset-1.60.1.71037`). Assets:

- `wowc-dataset-<build>.tar.zst`: the contents of `dist/<build>/` (the Parquet tables and `manifest.json`) in a directory named `<build>`.
- `manifest-<build>.json`: the same `manifest.json` as a separate download, so a consumer can read hashes without fetching the archive.

The release notes carry the licence and credits: the dataset is CC BY-SA 4.0; "Blizzard game client data, retrieved via wago.tools"; "Data from Everloot (https://everloot.app), CC BY-SA 4.0."

## When

The `dataset` workflow runs daily (06:17 UTC) and on manual dispatch. It asks wago.tools for the latest `wowf` build (`wowc-data release-check`); if `dataset-<build>` already exists it stops, otherwise it fetches, builds, runs `verify-manifest`, and publishes. A release is never rewritten.

## Verifying a download

```
mkdir data && tar --zstd -xf wowc-dataset-<build>.tar.zst -C data
go run github.com/WoW-Companion/data/cmd/wowc-data@latest verify-manifest data/<build>
```

`verify-manifest` checks `manifest.json` against its schema and recomputes each listed file's SHA-256. Without Go, compare by hand: every entry in `manifest.json`'s `tables` has a `path` and a `sha256`, so `shasum -a 256 data/<build>/<path>` must print that hash. Also check that `manifest-<build>.json` is identical to `data/<build>/manifest.json`.
