# Parquet layout spike

Measurements for the first two real tables, built from the client's tables for build
`1.60.1.71037` (product `wowf`). Re-run with:

```
uv run python -m wowc_data.build          # fetches once into dist/cache/, builds dist/tables/
uv run python spikes/parquet_layout/run.py
```

The script takes about 100 s. The HTTP cases need DuckDB's `httpfs` extension, which DuckDB
installs on first use, so the first run needs network.

Setup: DuckDB 2.0.1, pyarrow 26.0.1, zstd, one Apple-silicon laptop. Every case runs in its own
Python process so the peak RSS (`ru_maxrss`, whole process including the interpreter and DuckDB,
about 57 MB idle) belongs to that case alone. "http" is a small local static server with Range
support, so it measures request count and bytes, not network latency. "cold" means a fresh DuckDB
connection per lookup; the OS page cache is not dropped, so it understates a truly cold disk or
object store. Warm numbers reuse one connection.

## What the tables hold

- `items`: 19,400 rows from `ItemSparse` joined to `Item`. Columns: id, name, item level,
  required level, quality, class, subclass, slot (inventory type), allowable-class mask, bonding,
  stat type ids, and `stats_status` (`known`, `none` or `unknown`; 4,779, 14,125 and 496 rows).
  `unknown` marks gear (weapons and armor, uncommon or better, with a slot) for which the client
  lists no stats; those are never filled in.
- `quests`: 6,626 rows from `QuestV2`, with the quest line (`QuestLineXQuest`) and the first map
  (`QuestPOIBlob`) where they exist.

**The client's quest tables carry almost nothing.** `QuestV2` has only an id, a unique bit flag
and a UI theme id. Quest names, levels, minimum levels, zones and reward items are not in the
client tables; they are server-side data. Only 22 of the 6,626 quests have a quest line and a map.
So the quests table has no name, level, zone or reward columns, and the quest range query below
uses map and id range as stand-ins for zone and level band. Those figures say how the layout
behaves, not what a real quest query costs. `QuestPackageItem`, `QuestObjective` and
`QuestV2CliTask` do not exist in this build (HTTP 400, "Table not found").

## File sizes

Sorted by id, zstd.

(bytes, zstd, sorted by id)

| table | row group (k rows) | bloom | row groups | bytes |
|---|---|---|---|---|
| items | 4 | no | 5 | 354943 |
| items | 4 | yes | 5 | 428917 |
| items | 16 | no | 2 | 352787 |
| items | 16 | yes | 2 | 426611 |
| items | 64 | no | 1 | 354781 |
| items | 64 | yes | 1 | 420367 |
| quests | 4 | no | 2 | 53614 |
| quests | 4 | yes | 2 | 65950 |
| quests | 16 | no | 1 | 55078 |
| quests | 16 | yes | 1 | 63295 |
| quests | 64 | no | 1 | 55078 |
| quests | 64 | yes | 1 | 63295 |

## Point lookups by id

Lookups use `threads=1` (see the thread table below). 1,000 random ids for local warm, 200 for
http warm, 50 for local cold, 10 for http cold, to stay inside the run-time budget.

(ms; RSS = process peak MB, whole worker process)

| table | rg (k) | bloom | where | mode | n | p50 | p95 | RSS MB |
|---|---|---|---|---|---|---|---|---|
| items | 4 | no | local | cold | 50 | 18.25 | 30.91 | 59.39 |
| items | 4 | no | local | warm | 1000 | 2.23 | 4.06 | 60.39 |
| items | 4 | no | http | cold | 10 | 79.32 | 89.30 | 66.84 |
| items | 4 | no | http | warm | 200 | 5.20 | 9.31 | 68.77 |
| items | 4 | yes | local | cold | 50 | 17.52 | 25.10 | 59.19 |
| items | 4 | yes | local | warm | 1000 | 1.97 | 3.30 | 60.14 |
| items | 4 | yes | http | cold | 10 | 66.23 | 97.26 | 66.72 |
| items | 4 | yes | http | warm | 200 | 6.27 | 11.04 | 69.39 |
| items | 16 | no | local | cold | 50 | 15.70 | 27.07 | 59.92 |
| items | 16 | no | local | warm | 1000 | 2.90 | 4.39 | 60.86 |
| items | 16 | no | http | cold | 10 | 63.55 | 103.97 | 67.42 |
| items | 16 | no | http | warm | 200 | 6.68 | 10.90 | 69.33 |
| items | 16 | yes | local | cold | 50 | 17.58 | 23.62 | 59.78 |
| items | 16 | yes | local | warm | 1000 | 2.96 | 4.51 | 60.97 |
| items | 16 | yes | http | cold | 10 | 65.76 | 95.53 | 67.45 |
| items | 16 | yes | http | warm | 200 | 7.22 | 10.17 | 69.23 |
| items | 64 | no | local | cold | 50 | 16.50 | 19.32 | 59.78 |
| items | 64 | no | local | warm | 1000 | 3.13 | 4.36 | 60.55 |
| items | 64 | no | http | cold | 10 | 53.75 | 95.10 | 66.95 |
| items | 64 | no | http | warm | 200 | 7.18 | 10.19 | 68.98 |
| items | 64 | yes | local | cold | 50 | 17.81 | 24.49 | 59.59 |
| items | 64 | yes | local | warm | 1000 | 3.29 | 4.65 | 60.84 |
| items | 64 | yes | http | cold | 10 | 50.33 | 105.07 | 66.88 |
| items | 64 | yes | http | warm | 200 | 7.52 | 10.67 | 69.20 |
| quests | 4 | no | local | cold | 50 | 14.96 | 23.40 | 58.14 |
| quests | 4 | no | local | warm | 1000 | 0.94 | 1.47 | 59.31 |
| quests | 4 | no | http | cold | 10 | 43.71 | 69.88 | 65.75 |
| quests | 4 | no | http | warm | 200 | 3.43 | 5.76 | 67.70 |
| quests | 4 | yes | local | cold | 50 | 15.72 | 23.14 | 58.17 |
| quests | 4 | yes | local | warm | 1000 | 0.97 | 1.85 | 59.09 |
| quests | 4 | yes | http | cold | 10 | 79.95 | 98.91 | 65.84 |
| quests | 4 | yes | http | warm | 200 | 7.88 | 14.73 | 67.59 |
| quests | 16 | no | local | cold | 50 | 19.09 | 32.02 | 58.11 |
| quests | 16 | no | local | warm | 1000 | 1.21 | 2.06 | 58.97 |
| quests | 16 | no | http | cold | 10 | 54.19 | 66.64 | 65.53 |
| quests | 16 | no | http | warm | 200 | 5.00 | 7.69 | 67.36 |
| quests | 16 | yes | local | cold | 50 | 15.63 | 18.38 | 58.19 |
| quests | 16 | yes | local | warm | 1000 | 1.22 | 2.08 | 59.05 |
| quests | 16 | yes | http | cold | 10 | 53.20 | 84.16 | 65.66 |
| quests | 16 | yes | http | warm | 200 | 5.41 | 8.59 | 67.53 |
| quests | 64 | no | local | cold | 50 | 16.50 | 20.38 | 58.23 |
| quests | 64 | no | local | warm | 1000 | 1.39 | 2.48 | 58.94 |
| quests | 64 | no | http | cold | 10 | 79.32 | 108.21 | 65.72 |
| quests | 64 | no | http | warm | 200 | 5.88 | 13.31 | 67.31 |
| quests | 64 | yes | local | cold | 50 | 15.65 | 25.89 | 57.98 |
| quests | 64 | yes | local | warm | 1000 | 1.17 | 2.09 | 59.02 |
| quests | 64 | yes | http | cold | 10 | 49.02 | 117.17 | 65.47 |
| quests | 64 | yes | http | warm | 200 | 5.55 | 8.33 | 67.39 |

## Item lookups by exact name

Warm, 100 random distinct names, `threads=1`.

, warm (ms)

| rg (k) | bloom | where | n | p50 | p95 | RSS MB |
|---|---|---|---|---|---|---|
| 4 | no | local | 100 | 3.10 | 4.07 | 60.88 |
| 4 | no | http | 100 | 7.42 | 11.10 | 70.98 |
| 4 | yes | local | 100 | 2.02 | 3.37 | 60.52 |
| 4 | yes | http | 100 | 11.70 | 23.85 | 69.86 |
| 16 | no | local | 100 | 3.14 | 5.57 | 60.91 |
| 16 | no | http | 100 | 7.99 | 14.34 | 69.91 |
| 16 | yes | local | 100 | 3.03 | 4.16 | 60.53 |
| 16 | yes | http | 100 | 9.53 | 15.61 | 69.56 |
| 64 | no | local | 100 | 3.43 | 4.85 | 60.30 |
| 64 | no | http | 100 | 8.49 | 12.26 | 69.22 |
| 64 | yes | local | 100 | 3.43 | 5.01 | 60.56 |
| 64 | yes | http | 100 | 8.20 | 19.15 | 69.27 |

## DuckDB's thread pool

: default thread pool vs threads=1 (ms)

| table | rg (k) | threads | n | p50 | p95 |
|---|---|---|---|---|---|
| items | 4 | default | 100 | 11.80 | 13.19 |
| items | 4 | 1 | 100 | 2.06 | 3.51 |
| items | 16 | default | 100 | 11.89 | 12.43 |
| items | 16 | 1 | 100 | 2.92 | 4.15 |
| items | 64 | default | 100 | 3.10 | 4.30 |
| items | 64 | 1 | 100 | 3.24 | 4.16 |
| quests | 4 | default | 100 | 11.74 | 12.41 |
| quests | 4 | 1 | 100 | 0.87 | 1.48 |
| quests | 16 | default | 100 | 1.36 | 2.39 |
| quests | 16 | 1 | 100 | 1.12 | 1.77 |
| quests | 64 | default | 100 | 1.57 | 2.74 |
| quests | 64 | 1 | 100 | 1.01 | 1.56 |

## Range queries

Items: slot 5, level 40-60.
Quests: map 1, id 20k-60k.

| table | sort | groups kept by stats | where | p50 | p95 | RSS MB | HTTP requests (all runs) | HTTP bytes (all runs) |
|---|---|---|---|---|---|---|---|---|
| items | by_id | 5/5 | local | 2.02 | 3.15 | 59.00 |  |  |
| items | by_id | 5/5 | http | 208.07 | 352.77 | 70.64 | 132 | 1526932 |
| items | by_slot_level | 1/5 | local | 1.81 | 2.54 | 58.59 |  |  |
| items | by_slot_level | 1/5 | http | 5.29 | 6.72 | 67.11 | 44 | 438141 |
| items | unsorted | 5/5 | local | 1.95 | 2.65 | 59.33 |  |  |
| items | unsorted | 5/5 | http | 196.71 | 243.12 | 70.80 | 132 | 1766424 |
| quests | by_id | 1/2 | local | 1.23 | 1.89 | 57.50 |  |  |
| quests | by_id | 1/2 | http | 2.10 | 3.35 | 65.11 | 22 | 180224 |
| quests | by_map_id | 1/2 | local | 1.46 | 2.34 | 57.78 |  |  |
| quests | by_map_id | 1/2 | http | 4.90 | 7.18 | 65.72 | 33 | 515735 |
| quests | unsorted | 2/2 | local | 1.38 | 2.18 | 58.23 |  |  |
| quests | unsorted | 2/2 | http | 6.15 | 8.96 | 66.80 | 44 | 758131 |

"groups kept by stats" is how many row groups' min/max statistics can match the predicate. The
HTTP columns count requests and bytes over all 11 executions in one connection.

## Name search

| items | distinct names | distinct lower-case | avg length | distinct 3-char prefixes | distinct trigrams | trigram postings |
|---|---|---|---|---|---|---|
| 19400 | 18595 | 18595 | 21.96 | 1412 | 7097 | 369056 |

Names are not unique: 805 of 19,400 rows repeat the name of another item. Size of simple indexes,
as zstd Parquet: a prefix index (lower-cased name and id, sorted by name) is 193 KB, about half
the items table, and answers `like 'prefix%'` from row-group statistics. A trigram inverted index
(3-character grams of the lower-cased name to sorted id lists; 7,097 grams, 369,056 postings) is
532 KB, more than the table itself.

## Findings

- **At this size layout barely matters.** The whole items table is 350 KB and quests 55 KB. Row
  group size (4k, 16k, 64k) changes size by under 1% and warm lookup latency by about a
  millisecond. Items at 16k rows is 2 groups and quests is one group at 16k and 64k.
- **Bloom filters cost and gain nothing here.** They add about 20% to the file (items 353 to 427
  KB, quests 55 to 63 KB). Id lookups on id-sorted files are already pruned by min/max statistics,
  and exact-name lookups do not get faster except at 4k rows locally (3.1 to 2.0 ms), which is not
  repeated over http.
- **The default DuckDB thread pool adds about 10 ms to every point lookup on a multi-group
  file** (11.8 ms against 2 ms with `threads=1`, items at 4k and 16k). Single-group files do not
  show it. A point-lookup path should set `threads=1`.
- **Sorting matters for range queries over http, not locally.** Items by slot and level: sorted
  by (slot, item level, id) touches 1 of 5 groups, 44 requests and 438 KB in 5 ms; sorted by id or
  unsorted touches all 5 groups, 132 requests and 1.5 to 1.8 MB in 200 ms. Locally all three take
  about 2 ms. The quest case is too thin to say much (22 rows have a map).
- **http point lookups** cost 5 to 7 ms warm and 50 to 80 ms with a fresh connection, because the
  footer and group metadata are fetched again. RSS is 65 to 70 MB over http against 58 to 61 MB
  locally.
- **Downloading the whole table is the cheaper option at this size.** 350 KB plus 55 KB is one
  request each, which is fewer bytes than one unsorted range query.

## Recommendation

- One file per table per build, not split by column or by id range.
- Row group 16,384 rows. Equal on the numbers; it keeps groups few now and gives statistics
  pruning something to work with if a table grows to a few hundred thousand rows.
- Sort the primary file by `id`. Items also get a second copy sorted by (slot, item level, id) for
  range browsing, built at dataset time like the other precomputed joins. Quests need no second
  copy until they carry a level or zone.
- No Bloom filters for now; revisit when a table passes about a million rows or an equality
  column is not the sort key. The writer and the spike already support them.
- Readers use `threads=1` for point lookups.
- Name search: for `like 'prefix%'` use a 193 KB name-sorted copy of (lower-cased name, id); skip
  the trigram index (larger than the table) until substring search is required.
- Clients may download a table whole (under 0.5 MB each) instead of using range requests.

Limits of this spike: the tables are small, http was local, cold was not page-cache cold, and the
quest range case uses stand-in columns. None of that argues for a different layout at this size,
but the figures should be re-taken when NPCs and loot arrive.
