# Parquet layout spike

Measurements for the first two real tables, built from the client's tables for build
`1.60.1.71037` (product `wowf`). The advisor reads Parquet with parquet-go, so the layout is
decided on parquet-go numbers (first part). DATA-004's DuckDB numbers are kept as a reference
(second part); the two are not directly comparable, see "Reading the two sets side by side".

## parquet-go measurements

Re-run with:

```
make fetch        # once: client tables into dist/cache/ (needs network)
make spike        # Go benchmarks in spikes/parquet_layout/, about 45 s, offline
```

Results are also written to `dist/spike/results.md`.

Setup: parquet-go v0.32.1, Go 1.27.4, CGO off, zstd, one Apple-silicon laptop (M5), tables
built by `wowc-data build` code paths from the cached CSVs (items 19,400 rows, quests 6,626).
Everything runs in one process. "http" is an `httptest` server doing real HTTP Range requests,
one per `ReadAt` the library makes, so request counts and bytes are real and latency is loopback
only. "cold" opens the file afresh for every lookup (footer and page index read again); "warm"
reuses the open file. The OS page cache is not dropped, so cold understates a cold disk.
"heap MB" is the highest Go `HeapAlloc` sampled every 2 ms while the case ran; it includes
garbage not yet collected, so it is a noisy upper-ish figure and is not the same quantity as
DuckDB's process RSS.

Reader: a lookup prunes row groups by the id column's page-index min/max, then by the Bloom
filter when the file has one, reads only the one page of the id column that can hold the id,
then reads the row. Range queries prune groups by min/max of each predicate column and read the
rows of the groups that remain. Lookup counts match DATA-004's: 1,000 warm and 50 cold local,
200 warm and 10 cold http, 100 names.

### File sizes (parquet-go, zstd, sorted by id)

| table | row group (k rows) | bloom | row groups | bytes |
|---|---|---|---|---|
| items | 4 | no | 5 | 326646 |
| quests | 4 | no | 2 | 51800 |
| items | 4 | yes | 5 | 375388 |
| quests | 4 | yes | 2 | 60134 |
| items | 16 | no | 2 | 313059 |
| quests | 16 | no | 1 | 54859 |
| items | 16 | yes | 2 | 361667 |
| quests | 16 | yes | 1 | 63172 |
| items | 64 | no | 1 | 308475 |
| quests | 64 | no | 1 | 54859 |
| items | 64 | yes | 1 | 357037 |
| quests | 64 | yes | 1 | 63172 |

### Point lookups by id (ms; heap = peak Go heap MB while the case ran)

| table | rg (k) | bloom | where | mode | n | p50 | p95 | heap MB |
|---|---|---|---|---|---|---|---|---|
| items | 4 | no | local | cold | 50 | 0.68 | 1.43 | 124.3 |
| items | 4 | no | local | warm | 1000 | 0.54 | 2.00 | 33.9 |
| items | 4 | no | http | cold | 10 | 15.94 | 23.77 | 20.4 |
| items | 4 | no | http | warm | 200 | 6.07 | 15.10 | 25.9 |
| quests | 4 | no | local | cold | 50 | 0.65 | 2.29 | 19.0 |
| quests | 4 | no | local | warm | 1000 | 0.09 | 1.02 | 29.0 |
| quests | 4 | no | http | cold | 10 | 6.75 | 9.17 | 15.8 |
| quests | 4 | no | http | warm | 200 | 2.65 | 9.09 | 19.7 |
| items | 4 | yes | local | cold | 50 | 0.78 | 3.67 | 24.7 |
| items | 4 | yes | local | warm | 1000 | 0.54 | 1.95 | 30.1 |
| items | 4 | yes | http | cold | 10 | 8.89 | 11.83 | 21.6 |
| items | 4 | yes | http | warm | 200 | 4.62 | 7.84 | 25.1 |
| quests | 4 | yes | local | cold | 50 | 0.15 | 0.70 | 18.3 |
| quests | 4 | yes | local | warm | 1000 | 0.09 | 0.41 | 27.2 |
| quests | 4 | yes | http | cold | 10 | 7.32 | 9.80 | 16.2 |
| quests | 4 | yes | http | warm | 200 | 2.65 | 5.72 | 21.6 |
| items | 16 | no | local | cold | 50 | 2.11 | 3.97 | 29.0 |
| items | 16 | no | local | warm | 1000 | 1.55 | 3.70 | 32.3 |
| items | 16 | no | http | cold | 10 | 14.51 | 20.60 | 23.5 |
| items | 16 | no | http | warm | 200 | 9.05 | 16.32 | 29.5 |
| quests | 16 | no | local | cold | 50 | 0.22 | 1.18 | 24.2 |
| quests | 16 | no | local | warm | 1000 | 0.13 | 0.68 | 25.6 |
| quests | 16 | no | http | cold | 10 | 3.49 | 5.85 | 15.1 |
| quests | 16 | no | http | warm | 200 | 3.32 | 6.95 | 20.3 |
| items | 16 | yes | local | cold | 50 | 1.84 | 3.03 | 29.8 |
| items | 16 | yes | local | warm | 1000 | 1.81 | 4.49 | 31.9 |
| items | 16 | yes | http | cold | 10 | 17.25 | 26.85 | 23.1 |
| items | 16 | yes | http | warm | 200 | 10.89 | 20.33 | 30.9 |
| quests | 16 | yes | local | cold | 50 | 0.20 | 1.62 | 25.4 |
| quests | 16 | yes | local | warm | 1000 | 0.14 | 1.31 | 28.9 |
| quests | 16 | yes | http | cold | 10 | 11.44 | 17.12 | 15.4 |
| quests | 16 | yes | http | warm | 200 | 5.05 | 11.26 | 21.1 |
| items | 64 | no | local | cold | 50 | 2.25 | 6.24 | 28.1 |
| items | 64 | no | local | warm | 1000 | 1.94 | 5.42 | 34.8 |
| items | 64 | no | http | cold | 10 | 10.01 | 18.48 | 24.9 |
| items | 64 | no | http | warm | 200 | 8.21 | 17.35 | 30.8 |
| quests | 64 | no | local | cold | 50 | 0.20 | 1.18 | 23.0 |
| quests | 64 | no | local | warm | 1000 | 0.13 | 0.62 | 25.3 |
| quests | 64 | no | http | cold | 10 | 3.91 | 7.44 | 14.7 |
| quests | 64 | no | http | warm | 200 | 3.43 | 8.10 | 21.4 |
| items | 64 | yes | local | cold | 50 | 2.34 | 6.10 | 29.8 |
| items | 64 | yes | local | warm | 1000 | 1.97 | 4.54 | 33.4 |
| items | 64 | yes | http | cold | 10 | 12.29 | 19.92 | 24.9 |
| items | 64 | yes | http | warm | 200 | 10.04 | 17.71 | 32.2 |
| quests | 64 | yes | local | cold | 50 | 0.20 | 0.73 | 25.5 |
| quests | 64 | yes | local | warm | 1000 | 0.13 | 1.06 | 27.1 |
| quests | 64 | yes | http | cold | 10 | 5.82 | 14.07 | 15.6 |
| quests | 64 | yes | http | warm | 200 | 3.17 | 6.69 | 21.2 |

### Item lookups by exact name (warm, ms)

| rg (k) | bloom | where | n | p50 | p95 | heap MB |
|---|---|---|---|---|---|---|
| 4 | no | local | 100 | 1.11 | 4.08 | 27.8 |
| 4 | no | http | 100 | 7.73 | 15.55 | 25.4 |
| 4 | yes | local | 100 | 0.72 | 2.71 | 27.3 |
| 4 | yes | http | 100 | 8.40 | 19.16 | 24.4 |
| 16 | no | local | 100 | 2.64 | 7.73 | 32.8 |
| 16 | no | http | 100 | 13.01 | 26.92 | 29.4 |
| 16 | yes | local | 100 | 2.90 | 8.23 | 29.2 |
| 16 | yes | http | 100 | 11.03 | 19.55 | 29.1 |
| 64 | no | local | 100 | 2.23 | 5.18 | 28.7 |
| 64 | no | http | 100 | 9.30 | 16.18 | 30.4 |
| 64 | yes | local | 100 | 2.32 | 4.02 | 30.4 |
| 64 | yes | http | 100 | 11.88 | 22.54 | 30.6 |

### Range queries (items: slot 5, item level 40-60; quests: map 1, id 20000-60000)

| table | rg (k) | sort | groups kept by stats | rows | where | p50 ms | p95 ms | heap MB | HTTP requests (all runs) | HTTP bytes (all runs) |
|---|---|---|---|---|---|---|---|---|---|---|
| items | 4 | by_id | 5/5 | 147 | local | 28.93 | 34.05 | 42.9 |  |  |
| items | 4 | by_id | 5/5 | 147 | http | 59.22 | 74.85 | 19.5 | 836 | 3593106 |
| items | 4 | by_slot_level | 1/5 | 147 | local | 4.32 | 6.40 | 18.2 |  |  |
| items | 4 | by_slot_level | 1/5 | 147 | http | 11.36 | 23.72 | 18.4 | 220 | 781638 |
| quests | 4 | by_id | 1/2 | 0 | local | 1.77 | 2.93 | 13.9 |  |  |
| quests | 4 | by_id | 1/2 | 0 | http | 3.47 | 6.44 | 15.1 | 143 | 386309 |
| quests | 4 | by_map_id | 1/2 | 0 | local | 1.69 | 2.54 | 13.6 |  |  |
| quests | 4 | by_map_id | 1/2 | 0 | http | 5.91 | 7.50 | 15.1 | 143 | 302082 |
| items | 16 | by_id | 2/2 | 147 | local | 23.98 | 27.77 | 101.7 |  |  |
| items | 16 | by_id | 2/2 | 147 | http | 42.86 | 51.08 | 23.8 | 484 | 3443649 |
| items | 16 | by_slot_level | 1/2 | 147 | local | 18.37 | 26.11 | 21.6 |  |  |
| items | 16 | by_slot_level | 1/2 | 147 | http | 23.62 | 40.96 | 23.2 | 297 | 2609332 |
| quests | 16 | by_id | 1/1 | 0 | local | 4.16 | 7.95 | 20.3 |  |  |
| quests | 16 | by_id | 1/1 | 0 | http | 8.46 | 13.90 | 15.6 | 143 | 603449 |
| quests | 16 | by_map_id | 1/1 | 0 | local | 4.77 | 8.99 | 17.1 |  |  |
| quests | 16 | by_map_id | 1/1 | 0 | http | 8.09 | 10.61 | 16.4 | 143 | 474474 |

The quests range query matches 0 rows: only 22 quests have a map at all, and none is in map 1
with an id of 20,000 to 60,000. It still shows how many groups and bytes the pruning touches,
but says nothing about a real quest query. The range cases run at 4k and 16k rows per group (the
sort comparison needs more than one group).

### parquet-go findings

- **Files are smaller than DATA-004's.** Items 313 KB and quests 55 KB at 16k rows,
  against 353 KB and 55 KB in DATA-004. Row group size changes size by about 6% (items 327 KB
  at 4k, 308 KB at 64k).
- **Bloom filters still cost and gain nothing.** They add 15% to every file (items 313 to 362 KB
  at 16k, quests 55 to 63 KB). Id lookups on id-sorted files are already pruned by min/max, so
  the filter is a few extra bytes read for nothing (http warm items at 16k: 9.1 ms without, 10.9
  with). Exact-name lookups gain at 4k locally (1.1 to 0.7 ms) but not at 16k or 64k, and not
  over http.
- **Smaller groups are faster for items in parquet-go.** Warm local id lookup p50 on items is
  0.54 ms at 4k rows, 1.55 ms at 16k and 1.94 ms at 64k, and the same ordering holds over http
  (4.6 to 6.1 ms against 9 to 11 ms). The page that has to be decoded grows with the group, and
  parquet-go reads whole pages. Quests (one or two groups at every size) are 0.09 to 0.13 ms
  warm locally at any size. This reverses DATA-004's "16k, equal on the numbers", which came
  from DuckDB.
- **parquet-go has no thread-pool penalty.** DuckDB needed `threads=1` to avoid about 10 ms per
  lookup on multi-group files; there is nothing equivalent to set here.
- **Cold is cheap locally.** Local cold lookups are 0.2 to 2.3 ms; http cold is 3 to 17 ms (footer
  and page index fetched again) against 2.7 to 11 ms warm. Cold http items lookups are not
  faster at 4k rows (16 ms against 10 to 14 ms at 16k and 64k).
- **Sorting matters for range queries.** Items by (slot, item level, id) at 4k rows touch 1 of 5
  groups, 220 requests and 782 KB in 4.3 ms locally and 11 ms over http; sorted by id they touch
  5 of 5, 836 requests and 3.6 MB, 29 ms locally and 59 ms over http. At 16k rows both layouts
  touch most of the data (1 of 2 against 2 of 2 groups, 18 against 24 ms locally), so the
  secondary copy only pays off with small groups. The 11 executions per http case include the
  warm-up, as in DATA-004.
- **Heap is small.** Lookups peak at 15 to 35 MB of Go heap (garbage included); the range
  queries that read whole groups peak at 20 to 100 MB depending on when the collector ran.

### Reading the two sets side by side

DuckDB runs with a 57 MB idle process; parquet-go's figures are Go heap, not RSS. DuckDB's
latencies include SQL planning on every query; parquet-go's include only the hand-written
page-pruning reader above, which is what the advisor would ship. Treat the DuckDB figures as
what a SQL engine costs on the same files and the parquet-go figures as what the advisor's
reader costs; the layout choice below follows the second.

### Layout to adopt for the parquet-go reader

- One file per table per build, not split by column or by id range.
- **Row group 4,096 rows** (`parquetio.DefaultRowGroup`). Items are 5 groups, quests 2. It is
  the fastest for warm point lookups and for the sorted range query, at 4% more bytes than 16k.
  It is not best everywhere: cold http item lookups and the unsorted range scan are a little
  better at larger groups, and the differences are milliseconds.
- Sort the primary file by `id` (recorded in the file's sorting columns). Items also get a
  second copy sorted by (slot, item level, id) for range browsing, to be built at dataset time
  like the other precomputed joins; the pipeline does not build it yet.
- zstd, no Bloom filters (`tables.BuildDataset` writes none); revisit when a table passes about
  a million rows or a looked-up column is not the sort key.
- Provenance in the file's key-value metadata under `wowc.sources`.
- Clients may download a table whole (under 0.4 MB each) instead of using range requests.

Limits of this spike: the tables are small, http is loopback, cold is not page-cache cold, the
quest range case matches nothing, and the numbers come from one laptop and one run. Re-take them
when NPCs and loot arrive.

---

# DuckDB reference (DATA-004)

The measurements below are DATA-004's, taken with DuckDB 2.0.1 over files from the earlier writer
and left as they were reported. The script that produced them was removed from the repository
when the pipeline moved to Go; it is in the history of commit `7cf31b7`. They are kept for
comparison and no longer reproducible from this tree.
Setup: DuckDB 2.0.1, zstd, one Apple-silicon laptop. Every case ran in its own
process so the peak RSS (`ru_maxrss`, whole process including the interpreter and DuckDB,
about 57 MB idle) belongs to that case alone. "http" is a small local static server with Range
support, so it measures request count and bytes, not network latency. "cold" means a fresh DuckDB
connection per lookup; the OS page cache is not dropped, so it understates a truly cold disk or
object store. Warm numbers reuse one connection.

### What the tables hold

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

### File sizes

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

### Point lookups by id

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

### Item lookups by exact name

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

### DuckDB's thread pool

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

### Range queries

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

### Name search

| items | distinct names | distinct lower-case | avg length | distinct 3-char prefixes | distinct trigrams | trigram postings |
|---|---|---|---|---|---|---|
| 19400 | 18595 | 18595 | 21.96 | 1412 | 7097 | 369056 |

Names are not unique: 805 of 19,400 rows repeat the name of another item. Size of simple indexes,
as zstd Parquet: a prefix index (lower-cased name and id, sorted by name) is 193 KB, about half
the items table, and answers `like 'prefix%'` from row-group statistics. A trigram inverted index
(3-character grams of the lower-cased name to sorted id lists; 7,097 grams, 369,056 postings) is
532 KB, more than the table itself.

### Findings

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

### Recommendation

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
