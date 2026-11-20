import pyarrow as pa

from wowc_data.manifest import Source
from wowc_data.provenance import read_sources, stamp_sources


def test_stamp_round_trip_keeps_existing_metadata():
    table = pa.table({"id": [1, 2]}).replace_schema_metadata({"keep": "me"})
    srcs = [Source("client-tables", "1.0.0.12345", "2026-11-20T00:00:00Z", "CC BY-SA 4.0")]
    stamped = stamp_sources(table, srcs)
    assert read_sources(stamped) == srcs
    assert stamped.schema.metadata[b"keep"] == b"me"
    assert stamped.equals(table)


def test_unstamped_table_has_no_sources():
    assert read_sources(pa.table({"id": [1]})) == []
