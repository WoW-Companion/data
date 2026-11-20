"""Dataset manifest: typed model, JSON Schema validation and writer."""

from __future__ import annotations

import json
from dataclasses import asdict, dataclass, field
from pathlib import Path

from jsonschema import Draft202012Validator

SCHEMA_PATH = Path(__file__).resolve().parents[2] / "schemas" / "manifest.schema.json"


class ManifestError(ValueError):
    """Raised when a manifest does not satisfy the schema."""


@dataclass
class Source:
    name: str
    version: str  # the source's version or the game build it came from
    retrieved: str  # RFC 3339 timestamp
    license: str


@dataclass
class Table:
    path: str
    rows: int
    sha256: str
    sort_keys: list[str]
    sources: list[Source] = field(default_factory=list)


@dataclass
class Manifest:
    dataset_version: str
    game_build: str
    created: str  # RFC 3339 timestamp
    tables: list[Table] = field(default_factory=list)


def validate_manifest(data: dict) -> None:
    """Raise ManifestError listing every schema violation in `data`."""
    schema = json.loads(SCHEMA_PATH.read_text())
    errors = sorted(
        Draft202012Validator(schema).iter_errors(data), key=lambda e: list(e.absolute_path)
    )
    if errors:
        lines = [f"{'/'.join(map(str, e.absolute_path)) or '<root>'}: {e.message}" for e in errors]
        raise ManifestError("; ".join(lines))


def write_manifest(manifest: Manifest, path: Path) -> None:
    """Validate then write `manifest` as JSON; nothing is written if invalid."""
    data = asdict(manifest)
    validate_manifest(data)
    path.write_text(json.dumps(data, indent=2, sort_keys=True) + "\n")
