# Parquet layout

How the dataset's Parquet files are laid out and why. Measured with `wowc-data bench` (`make bench`)
on build `1.60.1.71037`, all 14 tables, 44,601 rows. It replaces the "Layout to adopt" section of
`docs/spikes/parquet-layout.md`, which measured only items and quests.

## Current layout

- One file per table per build. zstd. Sorted by the table's key columns (recorded in the file).
- Row group of 4,096 rows (`parquetio.DefaultRowGroup`). Only items (5 groups), quests (2) and
  recipe_reagents (3) have more than one.
- No Bloom filters, except `names.parquet` (below).
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

## names.parquet: the name index

`names.parquet` maps a normalised name to the row that carries it, so a reader finds a name without
scanning a table. Columns: `norm` (lowercased, accents folded, apostrophes dropped, other punctuation
a single space), `name` (as spelled), `table` (`items`, `npcs`, `instances`, `recipes`, `item_sets`) and
`id` (items.id, npcs.id, instances.map_id, recipes.spell_id, item_sets.set_id). Quests have no name in
the client tables and are not listed. Rows are sorted by `(norm, table, id)`; row groups hold 4,096 rows
and `norm` has a Bloom filter. Build 1.60.1.71037: 22,703 rows, 435,135 bytes, 6 row groups, 28,384
bytes of Bloom filter.

Row-group size was chosen by measuring 1,024, 2,048, 4,096 and 8,192 (HTTP, 20 ms per request, cold):

| row group | file bytes | exact lookup bytes | requests | HTTP p50 |
|---|---|---|---|---|
| 1,024 | 471,341 | 127,098 | 35.7 | 801 ms |
| 2,048 | 449,866 | 91,276 | 24.7 | 549 ms |
| 4,096 | 435,135 | 96,728 | 19.0 | 422 ms |
| 8,192 | 422,828 | 146,639 | 16.0 | 355 ms |

4,096 is the layout of every other table and the best balance of bytes and requests; 8,192 saves
requests but reads half as much again.

### Prefix and exact lookup

Read the footer, then walk the row groups using each `norm` column chunk's min and max
(`row_groups[i].columns[0].meta_data.statistics`, in the footer, so no extra request):

1. Prefix `p`: skip a group when `max < p`, or when `min > p` and `min` does not start with `p`. Read
   the rest in order from the first and stop at the first row that is greater than `p` and does not
   start with it. Matching names are adjacent because the file is sorted.
2. Exact `q`: the same min/max test, then check the group's Bloom filter on `norm` and skip the group
   when it says no.
3. `names.Lookup` in `internal/names` does both; `wowc-data bench` measures it.

Cost on 1.60.1.71037, file opened per lookup, 200 local and 20 HTTP lookups, 20 ms per request, prefix
of 4 characters returning at most 20 rows (14.7 on average):

| lookup | local p50 / p95 | HTTP p50 / p95 | local bytes | HTTP bytes | HTTP requests |
|---|---|---|---|---|---|
| exact | 0.35 / 0.78 ms | 421.57 / 446.10 ms | 97,838 | 96,728 | 19.0 |
| prefix | 0.38 / 1.02 ms | 399.65 / 464.10 ms | 97,805 | 96,696 | 18.0 |

The file is 435 kB, so a client that expects more than one lookup should download it once (one
request) and search it in memory.
