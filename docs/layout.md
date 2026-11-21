# Parquet layout

How the dataset's Parquet files are laid out and why. Measured with `wowc-data bench` (`make bench`)
on build `1.60.1.71037`, all 14 tables, 44,601 rows. It replaces the "Layout to adopt" section of
`docs/spikes/parquet-layout.md`, which measured only items and quests.

## Current layout

- One file per table per build. zstd. Sorted by the table's key columns (recorded in the file).
- Row group of 4,096 rows (`parquetio.DefaultRowGroup`). Only items (5 groups), quests (2) and
  recipe_reagents (3) have more than one.
- No Bloom filters.
- Dictionary encoding (`,dict` struct tag) on the low-cardinality columns of `items` (8 columns:
  required_level, quality, class_id, subclass_id, slot, allowable_class_mask, bonding,
  stats_status) and `loot_sources` (source_kind, tier, profession, everloot_data_as_of). Every
  other column is plain.
- Provenance in the key-value metadata under `wowc.sources`.

## What the bench measures

For every table: rows, file size, row groups, Bloom and dictionary columns; then p50 and p95 of a
point lookup by the leading sort key and of a 100-row range read by that key, local and over an
`httptest` HTTP range server that sleeps 20 ms per request (object storage), each cold (file
re-opened per lookup, footer and page index read again) and warm; plus bytes and requests per
lookup. Local runs 200 lookups per case, HTTP 20. The OS page cache is not dropped, so local cold
understates a cold disk. The full tables are in the DATA-021 report.

## What decides HTTP speed

A cold lookup is 8 to 31 range requests (footer, page indexes, then one page per column read for
the row), and each costs the 20 ms. Item point lookup: 30.8 requests, 658 ms cold; stat_types: 8
requests, 172 ms. Bytes barely matter at this size (items moves 106 kB per lookup). Every table is
under 0.41 MB, so a client that downloads the whole file pays one request.

## Tuning, before → after

Each variant was built from the same cache and benched with 100 local and 10 HTTP lookups. Local
and HTTP times vary about ±15% run to run (an unchanged build benched twice moved items cold HTTP
point from 690 to 646 ms), so only changes larger than that count as timing gains.

| change | before → after | verdict |
|---|---|---|
| Row group 4,096 → 16,384 | items 407,424 → 388,827 bytes (-4.6%), quests 53,828 → 56,265 (+4.5%); items point lookup 108 kB → 281 kB read, 0.28 → 0.82 ms local warm p50 | reverted: lookups read 2.6x the bytes, no net size gain |
| Row group 4,096 → 2,048 | items 407,424 → 435,182 bytes (+6.8%); point lookup read 108 kB → 76 kB; HTTP cold p50 690 → 642 ms (within noise) | reverted: no timing gain |
| Row group 4,096 → 1,024 | items 407,424 → 482,170 bytes (+18%); point lookup read 108 kB → 75 kB; HTTP cold p50 690 → 646 ms (within noise) | reverted: no timing gain |
| Bloom filter on each table's first sort key | items 407,424 → 431,795 bytes (+6%), quests 53,828 → 62,162 (+15%); requests per cold item lookup 30.4 → 36.4, HTTP cold p50 690 → 825 ms | reverted: slower and larger. The sorted key is already pruned by the page index; a Bloom filter costs an extra request |
| Dictionary on low-cardinality columns of every table | items -3.2%, loot_sources -6.0%, instances -0.7%, npcs +3.5%, quests +0.3%, recipes +0.1%, talent_builds +0.5%, talent_trees +1.7% | kept on items and loot_sources only; reverted on the rest, which grew |
| Dictionary on items and loot_sources (the kept change) | items 407,424 → 394,225 bytes; loot_sources 28,094 → 26,411; point lookup reads 108 → 105 kB (items), 35 → 33 kB (loot_sources); requests and times unchanged within noise | kept: smaller files, lookups no worse |

Dataset total: 619,729 → 604,847 bytes (-2.4%). Sort keys are unchanged: the leading key of each
table is its lookup key, and every point lookup reads one page of it.

## Not changed

- Bloom filters would pay off only for equality lookups on a column that is not the sort key
  (for example `items.name`). The bench looks up by sort key, so it does not measure that case.
- Page size was left at the library default.
