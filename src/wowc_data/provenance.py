"""Stamp a pyarrow table's schema metadata with the sources it came from."""

from __future__ import annotations

import json
from dataclasses import asdict

import pyarrow as pa

from wowc_data.manifest import Source

PROVENANCE_KEY = b"wowc.sources"


def stamp_sources(table: pa.Table, sources: list[Source]) -> pa.Table:
    """Return `table` with its sources recorded in schema metadata."""
    meta = dict(table.schema.metadata or {})
    meta[PROVENANCE_KEY] = json.dumps([asdict(s) for s in sources], sort_keys=True).encode()
    return table.replace_schema_metadata(meta)


def read_sources(table: pa.Table) -> list[Source]:
    """Read back the sources stamped by `stamp_sources`."""
    raw = (table.schema.metadata or {}).get(PROVENANCE_KEY)
    return [Source(**s) for s in json.loads(raw)] if raw else []
