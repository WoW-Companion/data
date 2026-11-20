"""Parquet layout spike: file sizes, point lookups and range queries for items and quests.

Run after `uv run python -m wowc_data.build`:

    uv run python spikes/parquet_layout/run.py

Writes variants under dist/spike/ and prints markdown tables. Needs DuckDB's httpfs
extension for the HTTP cases (installed on first use, so the first run needs network).
Re-runs in well under two minutes.
"""

from __future__ import annotations

import http.server
import json
import random
import re
import resource
import statistics
import subprocess
import sys
import threading
import time
from functools import partial
from pathlib import Path

ROW_GROUPS = (4096, 16384, 65536)
BLOOM = {
    "items": ["id", "name"],
    "quests": ["id"],
}
RANGE_SQL = {
    "items": "select count(*), min(id) from {src} where slot = 5 and item_level between 40 and 60",
    "quests": "select count(*), min(id) from {src} where map_id = 1 and id between 20000 and 60000",
}
RANGE_SORTS = {
    "items": {"by_id": ["id"], "by_slot_level": ["slot", "item_level", "id"]},
    "quests": {"by_id": ["id"], "by_map_id": ["map_id", "id"]},
}
LOOKUPS_WARM = 1000
LOOKUPS_NAMES = 100
LOOKUPS_COLD = 50
LOOKUPS_HTTP_COLD = 10
LOOKUPS_HTTP_WARM = 200
POINT_THREADS = 1  # see the threads table: the default pool adds ~10 ms to multi-row-group files
RANGE_RUNS = 10
SEED = 7


# ---------------------------------------------------------------- worker (per process RSS)


def pct(xs: list[float], p: float) -> float:
    xs = sorted(xs)
    return xs[min(len(xs) - 1, int(p * len(xs)))]


def worker(spec: dict) -> dict:
    import duckdb

    def connect():
        c = duckdb.connect()
        if spec["url"]:
            c.execute("load httpfs")
        if spec.get("threads"):
            c.execute(f"set threads={spec['threads']}")
        return c

    src = f"'{spec['src']}'"
    out: dict = {}
    if spec["kind"] == "range":
        sql = RANGE_SQL[spec["table"]].format(src=src)
        c = connect()
        c.execute(sql).fetchall()
        times = []
        for _ in range(RANGE_RUNS):
            t = time.perf_counter()
            c.execute(sql).fetchall()
            times.append((time.perf_counter() - t) * 1000)
        out["p50_ms"], out["p95_ms"] = statistics.median(times), pct(times, 0.95)
    else:
        col, keys = spec["col"], spec["keys"]
        times = []
        if spec["mode"] == "cold":
            for k in keys:
                t = time.perf_counter()
                c = connect()
                c.execute(f"select * from {src} where {col} = ?", [k]).fetchall()
                times.append((time.perf_counter() - t) * 1000)
                c.close()
        else:
            c = connect()
            c.execute(f"select * from {src} where {col} = ?", [keys[0]]).fetchall()
            for k in keys:
                t = time.perf_counter()
                c.execute(f"select * from {src} where {col} = ?", [k]).fetchall()
                times.append((time.perf_counter() - t) * 1000)
        out["p50_ms"], out["p95_ms"] = statistics.median(times), pct(times, 0.95)
    out["rss_mb"] = resource.getrusage(resource.RUSAGE_SELF).ru_maxrss / 1024 / 1024  # macOS bytes
    if sys.platform != "darwin":
        out["rss_mb"] = resource.getrusage(resource.RUSAGE_SELF).ru_maxrss / 1024
    return out


# ---------------------------------------------------------------- range-capable static server


class Stats:
    requests = 0
    bytes = 0


class RangeHandler(http.server.SimpleHTTPRequestHandler):
    def log_message(self, *a):
        pass

    def send_head(self):
        path = Path(self.translate_path(self.path))
        if not path.is_file():
            self.send_error(404)
            return None
        size = path.stat().st_size
        m = re.match(r"bytes=(\d*)-(\d*)", self.headers.get("Range", ""))
        f = path.open("rb")
        if m and (m.group(1) or m.group(2)):
            if m.group(1):
                start = int(m.group(1))
                end = int(m.group(2)) if m.group(2) else size - 1
            else:
                start, end = max(0, size - int(m.group(2))), size - 1
            end = min(end, size - 1)
            self.send_response(206)
            self.send_header("Content-Range", f"bytes {start}-{end}/{size}")
            self.send_header("Content-Length", str(end - start + 1))
            f.seek(start)
            self._remaining = end - start + 1
        else:
            self.send_response(200)
            self.send_header("Content-Length", str(size))
            self._remaining = size
        self.send_header("Accept-Ranges", "bytes")
        self.send_header("Content-Type", "application/octet-stream")
        try:
            self.end_headers()
        except (BrokenPipeError, ConnectionResetError):
            return None
        Stats.requests += 1
        return f

    def copyfile(self, source, outputfile):
        data = source.read(self._remaining)
        Stats.bytes += len(data)
        try:
            outputfile.write(data)
        except (BrokenPipeError, ConnectionResetError):
            pass


def serve(directory: Path) -> http.server.ThreadingHTTPServer:
    handler = partial(RangeHandler, directory=str(directory))
    srv = http.server.ThreadingHTTPServer(("127.0.0.1", 0), handler)
    threading.Thread(target=srv.serve_forever, daemon=True).start()
    return srv


# ---------------------------------------------------------------- build variants


def write_variant(table, path: Path, rg: int, bloom_cols: list[str], sort: list[str] | None):
    import pyarrow.compute as pc
    import pyarrow.parquet as pq

    if sort is None:  # unsorted: a fixed shuffle
        idx = list(range(table.num_rows))
        random.Random(SEED).shuffle(idx)
        t = table.take(idx)
    else:
        t = table.sort_by([(c, "ascending") for c in sort])
    del pc
    kw = {}
    if bloom_cols:
        kw["bloom_filter_options"] = {
            c: {"ndv": min(rg, t.num_rows), "fpp": 0.01} for c in bloom_cols
        }
    pq.write_table(t, path, compression="zstd", row_group_size=rg, write_statistics=True, **kw)


def groups_touched(path: Path, table: str) -> tuple[int, int]:
    """Row groups whose min/max statistics could match the range predicate."""
    import pyarrow.parquet as pq

    md = pq.ParquetFile(path).metadata
    cols = {md.schema.column(i).name: i for i in range(md.num_columns)}
    if table == "items":
        preds = {"slot": (5, 5), "item_level": (40, 60)}
    else:
        preds = {"map_id": (1, 1), "id": (20000, 60000)}
    hit = 0
    for g in range(md.num_row_groups):
        ok = True
        for name, (lo, hi) in preds.items():
            st = md.row_group(g).column(cols[name]).statistics
            if st is None or not st.has_min_max:
                continue
            if st.max < lo or st.min > hi:
                ok = False
        hit += ok
    return hit, md.num_row_groups


def run_worker(spec: dict) -> dict:
    p = subprocess.run(
        [sys.executable, __file__, "--worker", json.dumps(spec)],
        capture_output=True,
        text=True,
        check=True,
    )
    return json.loads(p.stdout.strip().splitlines()[-1])


def md(headers: list[str], rows: list[list]) -> str:
    def f(v):
        return f"{v:.2f}" if isinstance(v, float) else str(v)

    lines = ["| " + " | ".join(headers) + " |", "|" + "|".join("---" for _ in headers) + "|"]
    lines += ["| " + " | ".join(f(v) for v in r) + " |" for r in rows]
    return "\n".join(lines)


def main() -> None:
    import duckdb
    import pyarrow.parquet as pq

    t0 = time.perf_counter()
    tables_dir = Path("dist/tables")
    out_dir = Path("dist/spike")
    out_dir.mkdir(parents=True, exist_ok=True)
    for old in out_dir.glob("*.parquet"):
        old.unlink()
    tables = {n: pq.read_table(tables_dir / f"{n}.parquet") for n in ("items", "quests")}
    ids = {n: t.column("id").to_pylist() for n, t in tables.items()}
    rng = random.Random(SEED)
    keys = {n: [rng.choice(v) for _ in range(LOOKUPS_WARM)] for n, v in ids.items()}
    names = rng.sample(sorted(set(tables["items"].column("name").to_pylist())), LOOKUPS_NAMES)
    srv = serve(out_dir)
    base = f"http://127.0.0.1:{srv.server_address[1]}"

    # 1. sizes (sorted by id, the primary layout)
    size_rows = []
    for n, t in tables.items():
        for rg in ROW_GROUPS:
            for bloom in (False, True):
                p = out_dir / f"{n}_rg{rg // 1024}k_{'bloom' if bloom else 'nobloom'}.parquet"
                write_variant(t, p, rg, BLOOM[n] if bloom else [], ["id"])
                ngroups = pq.ParquetFile(p).metadata.num_row_groups
                size_rows.append(
                    [n, rg // 1024, "yes" if bloom else "no", ngroups, p.stat().st_size]
                )
    print("## File sizes (bytes, zstd, sorted by id)\n")
    print(md(["table", "row group (k rows)", "bloom", "row groups", "bytes"], size_rows))

    print(f"[{time.perf_counter() - t0:.0f}s] before # 2. point", file=sys.stderr)
    # 2. point lookups
    lk_rows = []
    for n in tables:
        for rg in ROW_GROUPS:
            for bloom in (False, True):
                name = f"{n}_rg{rg // 1024}k_{'bloom' if bloom else 'nobloom'}.parquet"
                for where, src, url in (
                    ("local", str(out_dir / name), False),
                    ("http", f"{base}/{name}", True),
                ):
                    for mode in ("cold", "warm"):
                        if mode == "cold":
                            n_keys = LOOKUPS_HTTP_COLD if where == "http" else LOOKUPS_COLD
                        else:
                            n_keys = LOOKUPS_HTTP_WARM if where == "http" else LOOKUPS_WARM
                        ks = keys[n][:n_keys]
                        r = run_worker(
                            {
                                "kind": "point",
                                "table": n,
                                "src": src,
                                "url": url,
                                "col": "id",
                                "mode": mode,
                                "keys": ks,
                                "threads": POINT_THREADS,
                            }
                        )
                        lk_rows.append(
                            [
                                n,
                                rg // 1024,
                                "yes" if bloom else "no",
                                where,
                                mode,
                                len(ks),
                                r["p50_ms"],
                                r["p95_ms"],
                                r["rss_mb"],
                            ]
                        )
    print("\n## Point lookups by id (ms; RSS = process peak MB, whole worker process)\n")
    print(md(["table", "rg (k)", "bloom", "where", "mode", "n", "p50", "p95", "RSS MB"], lk_rows))

    print(f"[{time.perf_counter() - t0:.0f}s] before # 2b.", file=sys.stderr)
    # 2b. lookup by name (items)
    nm_rows = []
    for rg in ROW_GROUPS:
        for bloom in (False, True):
            name = f"items_rg{rg // 1024}k_{'bloom' if bloom else 'nobloom'}.parquet"
            for where, src, url in (
                ("local", str(out_dir / name), False),
                ("http", f"{base}/{name}", True),
            ):
                r = run_worker(
                    {
                        "kind": "point",
                        "table": "items",
                        "src": src,
                        "url": url,
                        "col": "name",
                        "mode": "warm",
                        "keys": names,
                        "threads": POINT_THREADS,
                    }
                )
                nm_rows.append(
                    [
                        rg // 1024,
                        "yes" if bloom else "no",
                        where,
                        len(names),
                        r["p50_ms"],
                        r["p95_ms"],
                        r["rss_mb"],
                    ]
                )
    print("\n## Item lookups by exact name, warm (ms)\n")
    print(md(["rg (k)", "bloom", "where", "n", "p50", "p95", "RSS MB"], nm_rows))

    print(f"[{time.perf_counter() - t0:.0f}s] before # 2c.", file=sys.stderr)
    # 2c. DuckDB thread pool effect on warm local lookups
    th_rows = []
    for n in tables:
        for rg in ROW_GROUPS:
            src = str(out_dir / f"{n}_rg{rg // 1024}k_nobloom.parquet")
            for th in (None, 1):
                r = run_worker(
                    {
                        "kind": "point",
                        "table": n,
                        "src": src,
                        "url": False,
                        "col": "id",
                        "mode": "warm",
                        "keys": keys[n][:LOOKUPS_NAMES],
                        "threads": th,
                    }
                )
                th_rows.append(
                    [n, rg // 1024, th or "default", LOOKUPS_NAMES, r["p50_ms"], r["p95_ms"]]
                )
    print("\n## Warm local lookups by id: default thread pool vs threads=1 (ms)\n")
    print(md(["table", "rg (k)", "threads", "n", "p50", "p95"], th_rows))

    print(f"[{time.perf_counter() - t0:.0f}s] before # 3. range", file=sys.stderr)
    # 3. range queries at 4k rows per group
    rg_rows = []
    for n, t in tables.items():
        variants = dict(RANGE_SORTS[n]) | {"unsorted": None}
        for label, sort in variants.items():
            p = out_dir / f"{n}_range_{label}.parquet"
            write_variant(t, p, 4096, [], sort)
            hit, total = groups_touched(p, n)
            for where, src, url in (("local", str(p), False), ("http", f"{base}/{p.name}", True)):
                Stats.requests = Stats.bytes = 0
                r = run_worker(
                    {"kind": "range", "table": n, "src": src, "url": url, "threads": POINT_THREADS}
                )
                extra = (Stats.requests, Stats.bytes) if url else ("", "")
                rg_rows.append(
                    [
                        n,
                        label,
                        f"{hit}/{total}",
                        where,
                        r["p50_ms"],
                        r["p95_ms"],
                        r["rss_mb"],
                        extra[0],
                        extra[1],
                    ]
                )
    print(
        "\n## Range queries (4k-row groups)\n\n"
        "Items: slot 5, level 40-60.\nQuests: map 1, id 20k-60k.\n"
    )
    print(
        md(
            [
                "table",
                "sort",
                "groups kept by stats",
                "where",
                "p50",
                "p95",
                "RSS MB",
                "HTTP requests (all runs)",
                "HTTP bytes (all runs)",
            ],
            rg_rows,
        )
    )

    print(f"[{time.perf_counter() - t0:.0f}s] before # 4. name", file=sys.stderr)
    # 4. name search notes
    con = duckdb.connect()
    d = con.execute(
        "select count(*), count(distinct name), count(distinct lower(name)), avg(length(name)) "
        f"from '{tables_dir}/items.parquet'"
    ).fetchone()
    names_all = [x.lower() for x in tables["items"].column("name").to_pylist()]
    prefixes = {x[:3] for x in names_all}
    trigrams = {x[i : i + 3] for x in names_all for i in range(max(1, len(x) - 2))}
    postings = sum(len({x[i : i + 3] for i in range(max(1, len(x) - 2))}) for x in set(names_all))
    print("\n## Name search\n")
    print(
        md(
            [
                "items",
                "distinct names",
                "distinct lower-case",
                "avg length",
                "distinct 3-char prefixes",
                "distinct trigrams",
                "trigram postings",
            ],
            [[d[0], d[1], d[2], d[3], len(prefixes), len(trigrams), postings]],
        )
    )
    srv.shutdown()
    print(f"\ntotal {time.perf_counter() - t0:.1f}s", file=sys.stderr)


if __name__ == "__main__":
    if len(sys.argv) > 2 and sys.argv[1] == "--worker":
        print(json.dumps(worker(json.loads(sys.argv[2]))))
    else:
        main()
