import copy

import pytest

from wowc_data.manifest import (
    Manifest,
    ManifestError,
    Source,
    Table,
    validate_manifest,
    write_manifest,
)

VALID = {
    "dataset_version": "1.0.0",
    "game_build": "1.0.0.12345",
    "created": "2026-11-20T00:00:00Z",
    "tables": [
        {
            "path": "items.parquet",
            "rows": 3,
            "sha256": "a" * 64,
            "sort_keys": ["item_id"],
            "sources": [
                {
                    "name": "client-tables",
                    "version": "1.0.0.12345",
                    "retrieved": "2026-11-20T00:00:00Z",
                    "license": "CC BY-SA 4.0",
                }
            ],
        }
    ],
}


def without(path):
    data = copy.deepcopy(VALID)
    node = data
    for key in path[:-1]:
        node = node[key]
    del node[path[-1]]
    return data


MISSING = [
    ["dataset_version"],
    ["game_build"],
    ["created"],
    ["tables"],
    ["tables", 0, "path"],
    ["tables", 0, "rows"],
    ["tables", 0, "sha256"],
    ["tables", 0, "sort_keys"],
    ["tables", 0, "sources"],
    ["tables", 0, "sources", 0, "name"],
    ["tables", 0, "sources", 0, "version"],
    ["tables", 0, "sources", 0, "retrieved"],
    ["tables", 0, "sources", 0, "license"],
]


def test_valid_manifest_passes():
    validate_manifest(VALID)


@pytest.mark.parametrize("path", MISSING, ids=lambda p: "/".join(map(str, p)))
def test_missing_required_field_fails(path):
    with pytest.raises(ManifestError):
        validate_manifest(without(path))


def test_bad_sha_fails():
    data = copy.deepcopy(VALID)
    data["tables"][0]["sha256"] = "xyz"
    with pytest.raises(ManifestError):
        validate_manifest(data)


def test_write_manifest(tmp_path):
    src = Source("client-tables", "1.0.0.12345", "2026-11-20T00:00:00Z", "CC BY-SA 4.0")
    m = Manifest(
        "1.0.0",
        "1.0.0.12345",
        "2026-11-20T00:00:00Z",
        [Table("items.parquet", 3, "a" * 64, ["item_id"], [src])],
    )
    out = tmp_path / "manifest.json"
    write_manifest(m, out)
    assert '"game_build": "1.0.0.12345"' in out.read_text()


def test_write_invalid_writes_nothing(tmp_path):
    out = tmp_path / "manifest.json"
    with pytest.raises(ManifestError):
        write_manifest(Manifest("", "b", "2026-11-20T00:00:00Z"), out)
    assert not out.exists()
