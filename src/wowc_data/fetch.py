"""Fetch client tables from wago.tools' CSV export into the local cache, once per build."""

from __future__ import annotations

import json
import time
import urllib.parse
import urllib.request
from datetime import UTC, datetime
from pathlib import Path

BASE_URL = "https://wago.tools"
PRODUCT = "wowf"
CREDIT = "Blizzard game client data, retrieved via wago.tools"
LICENSE_BASIS = (
    "Operator ruling 2026-11-20: client tables retrieved via wago.tools may be used; "
    "facts only, no long-form text"
)
USER_AGENT = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 Safari/605.1.15"
TABLES = (
    "ItemSparse",
    "Item",
    "QuestV2",
    "QuestLineXQuest",
    "QuestPOIBlob",
)
DELAY_SECONDS = 2.0
PROVENANCE_FILE = "provenance.json"


def table_url(table: str, build: str) -> str:
    return f"{BASE_URL}/db2/{table}/csv?" + urllib.parse.urlencode({"version": build})


def make_provenance(build: str, retrieved: str, tables: list[str]) -> dict:
    """The record kept beside the cached CSVs and stamped into every built table."""
    return {
        "source": "wago.tools",
        "credit": CREDIT,
        "product": PRODUCT,
        "build": build,
        "retrieved": retrieved,
        "license": LICENSE_BASIS,
        "tables": sorted(tables),
    }


def fetch_build(build: str, cache_dir: Path, tables: tuple[str, ...] = TABLES) -> dict:
    """Download `tables` for `build` one at a time (no parallelism), skipping cached files."""
    raw = cache_dir / build
    raw.mkdir(parents=True, exist_ok=True)
    prov_path = raw / PROVENANCE_FILE
    prov = json.loads(prov_path.read_text()) if prov_path.exists() else None
    fetched = False
    for table in tables:
        dest = raw / f"{table}.csv"
        if dest.exists():
            continue
        if fetched:
            time.sleep(DELAY_SECONDS)
        req = urllib.request.Request(table_url(table, build), headers={"User-Agent": USER_AGENT})
        with urllib.request.urlopen(req, timeout=120) as resp:  # noqa: S310
            dest.write_bytes(resp.read())
        fetched = True
    if prov is None or fetched:
        retrieved = datetime.now(UTC).strftime("%Y-%m-%dT%H:%M:%SZ")
        if prov is not None:
            retrieved = prov["retrieved"]
        prov = make_provenance(build, retrieved, list(tables))
        prov_path.write_text(json.dumps(prov, indent=2, sort_keys=True) + "\n")
    return prov
