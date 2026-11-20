"""Build the `items` and `quests` tables from cached client CSVs.

Only facts are kept: ids, numbers, flags, slots and names. Long-form text columns
(item descriptions, quest text) are never read into the output.
"""

from __future__ import annotations

import csv
import json
from pathlib import Path

import pyarrow as pa

from wowc_data.manifest import Source
from wowc_data.provenance import stamp_sources

STAT_SLOTS = 10
# Item classes whose equippable pieces carry stats: weapons (2) and armor (4).
GEAR_CLASSES = {2, 4}
# Quality 2 (uncommon) and up is expected to carry stats when equippable.
MIN_STATTED_QUALITY = 2

STATS_KNOWN = "known"
STATS_NONE = "none"
STATS_UNKNOWN = "unknown"

ITEM_SCHEMA = pa.schema(
    [
        ("id", pa.int32()),
        ("name", pa.string()),
        ("item_level", pa.int16()),
        ("required_level", pa.int16()),
        ("quality", pa.int8()),
        ("class_id", pa.int8()),
        ("subclass_id", pa.int8()),
        ("slot", pa.int8()),
        ("allowable_class_mask", pa.int32()),
        ("bonding", pa.int8()),
        ("stat_types", pa.list_(pa.int16())),
        ("stats_status", pa.string()),
    ]
)

QUEST_SCHEMA = pa.schema(
    [
        ("id", pa.int32()),
        ("unique_bit_flag", pa.int32()),
        ("ui_theme_id", pa.int32()),
        ("quest_line_id", pa.int32()),
        ("map_id", pa.int32()),
        ("ui_map_id", pa.int32()),
    ]
)


def read_csv(path: Path) -> list[dict[str, str]]:
    with path.open(newline="", encoding="utf-8") as f:
        return list(csv.DictReader(f))


def _int(row: dict[str, str], key: str, default: int = 0) -> int:
    v = row.get(key, "")
    return int(v) if v not in ("", None) else default


def stat_types(row: dict[str, str]) -> list[int]:
    """Stat type ids the client lists for an item (-1 marks an empty slot)."""
    return [
        s for i in range(STAT_SLOTS) if (s := _int(row, f"StatModifier_bonusStat_{i}", -1)) != -1
    ]


def stats_status(stats: list[int], class_id: int, slot: int, quality: int) -> str:
    """`known` when stats are listed; `unknown` when a stat-bearing piece lists none."""
    if stats:
        return STATS_KNOWN
    if class_id in GEAR_CLASSES and slot != 0 and quality >= MIN_STATTED_QUALITY:
        return STATS_UNKNOWN
    return STATS_NONE


def build_items(sparse: list[dict[str, str]], items: list[dict[str, str]]) -> pa.Table:
    base = {_int(r, "ID"): r for r in items}
    rows = []
    for r in sparse:
        item_id = _int(r, "ID")
        b = base.get(item_id, {})
        stats = stat_types(r)
        class_id = _int(b, "ClassID")
        slot = _int(r, "InventoryType")
        quality = _int(r, "OverallQualityID")
        rows.append(
            {
                "id": item_id,
                "name": r.get("Display_lang", ""),
                "item_level": _int(r, "ItemLevel"),
                "required_level": _int(r, "RequiredLevel"),
                "quality": quality,
                "class_id": class_id,
                "subclass_id": _int(b, "SubclassID"),
                "slot": slot,
                "allowable_class_mask": _int(r, "AllowableClass", -1),
                "bonding": _int(r, "Bonding"),
                "stat_types": stats,
                "stats_status": stats_status(stats, class_id, slot, quality),
            }
        )
    rows.sort(key=lambda x: x["id"])
    return pa.Table.from_pylist(rows, schema=ITEM_SCHEMA)


def build_quests(
    quests: list[dict[str, str]],
    lines: list[dict[str, str]],
    pois: list[dict[str, str]],
) -> pa.Table:
    """The client's quest tables carry no name, level, zone text or rewards; keep what exists."""
    line_of: dict[int, int] = {}
    for r in sorted(lines, key=lambda r: _int(r, "ID")):
        line_of.setdefault(_int(r, "QuestID"), _int(r, "QuestLineID"))
    poi_of: dict[int, dict[str, str]] = {}
    for r in sorted(pois, key=lambda r: _int(r, "ID")):
        poi_of.setdefault(_int(r, "QuestID"), r)
    out = []
    for r in quests:
        qid = _int(r, "ID")
        poi = poi_of.get(qid)
        out.append(
            {
                "id": qid,
                "unique_bit_flag": _int(r, "UniqueBitFlag"),
                "ui_theme_id": _int(r, "UiQuestDetailsThemeID"),
                "quest_line_id": line_of.get(qid),
                "map_id": _int(poi, "MapID") if poi else None,
                "ui_map_id": _int(poi, "UiMapID") if poi else None,
            }
        )
    out.sort(key=lambda x: x["id"])
    return pa.Table.from_pylist(out, schema=QUEST_SCHEMA)


def source_from(prov: dict) -> Source:
    return Source(
        name=f"{prov['source']} ({prov['product']}: {', '.join(prov['tables'])})",
        version=prov["build"],
        retrieved=prov["retrieved"],
        license=prov["license"],
    )


def build_all(cache_build_dir: Path) -> dict[str, pa.Table]:
    """Build both tables from a cache directory, stamped with the cache's provenance."""
    prov = json.loads((cache_build_dir / "provenance.json").read_text())
    src = [source_from(prov)]
    items = build_items(
        read_csv(cache_build_dir / "ItemSparse.csv"), read_csv(cache_build_dir / "Item.csv")
    )
    quests = build_quests(
        read_csv(cache_build_dir / "QuestV2.csv"),
        read_csv(cache_build_dir / "QuestLineXQuest.csv"),
        read_csv(cache_build_dir / "QuestPOIBlob.csv"),
    )
    return {"items": stamp_sources(items, src), "quests": stamp_sources(quests, src)}
