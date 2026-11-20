"""Command line: fetch (once) and build the items and quests tables into dist/."""

from __future__ import annotations

import argparse
from pathlib import Path

import pyarrow.parquet as pq

from wowc_data.fetch import fetch_build
from wowc_data.tables import build_all

DEFAULT_BUILD = "1.60.1.71037"


def main(argv: list[str] | None = None) -> None:
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--build", default=DEFAULT_BUILD)
    ap.add_argument("--cache", type=Path, default=Path("dist/cache"))
    ap.add_argument("--out", type=Path, default=Path("dist/tables"))
    args = ap.parse_args(argv)
    fetch_build(args.build, args.cache)
    args.out.mkdir(parents=True, exist_ok=True)
    for name, table in build_all(args.cache / args.build).items():
        pq.write_table(table, args.out / f"{name}.parquet", compression="zstd")
        print(f"{name}: {table.num_rows} rows")


if __name__ == "__main__":
    main()
