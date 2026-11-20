import pytest

from wowc_data.fetch import CREDIT, make_provenance, table_url
from wowc_data.provenance import read_sources
from wowc_data.tables import (
    build_all,
    build_items,
    build_quests,
    stat_types,
    stats_status,
)

SPARSE = [
    {
        "ID": "25",
        "Display_lang": "Worn Shortsword",
        "Description_lang": "flavour text",
        "ItemLevel": "1",
        "RequiredLevel": "0",
        "OverallQualityID": "1",
        "InventoryType": "21",
        "AllowableClass": "-1",
        "Bonding": "0",
        **{f"StatModifier_bonusStat_{i}": "-1" for i in range(10)},
    },
    {
        "ID": "10",
        "Display_lang": "Mystery Helm",
        "ItemLevel": "20",
        "RequiredLevel": "15",
        "OverallQualityID": "3",
        "InventoryType": "1",
        "AllowableClass": "32767",
        "Bonding": "1",
        **{f"StatModifier_bonusStat_{i}": "-1" for i in range(10)},
    },
    {
        "ID": "11",
        "Display_lang": "Known Helm",
        "OverallQualityID": "3",
        "InventoryType": "1",
        "StatModifier_bonusStat_0": "3",
        "StatModifier_bonusStat_1": "7",
        "StatModifier_bonusStat_2": "-1",
    },
]
ITEMS = [
    {"ID": "25", "ClassID": "2", "SubclassID": "7"},
    {"ID": "10", "ClassID": "4", "SubclassID": "1"},
    {"ID": "11", "ClassID": "4", "SubclassID": "1"},
]


@pytest.mark.parametrize(
    ("stats", "class_id", "slot", "quality", "want"),
    [
        ([3], 4, 1, 3, "known"),
        ([], 4, 1, 3, "unknown"),
        ([], 2, 21, 2, "unknown"),
        ([], 2, 21, 1, "none"),
        ([], 4, 0, 3, "none"),
        ([], 0, 0, 3, "none"),
    ],
)
def test_stats_status(stats, class_id, slot, quality, want):
    assert stats_status(stats, class_id, slot, quality) == want


def test_stat_types_skips_empty_slots():
    assert stat_types(SPARSE[2]) == [3, 7]
    assert stat_types({}) == []


def test_build_items_sorted_and_no_long_text():
    t = build_items(SPARSE, ITEMS)
    assert t.column("id").to_pylist() == [10, 11, 25]
    assert "description" not in " ".join(t.column_names)
    assert t.column("stats_status").to_pylist() == ["unknown", "known", "none"]
    assert t.column("class_id").to_pylist() == [4, 4, 2]
    assert t.column("stat_types").to_pylist() == [[], [3, 7], []]


def test_build_quests_joins_line_and_poi():
    quests = [{"ID": "2", "UniqueBitFlag": "5"}, {"ID": "1", "UniqueBitFlag": "6"}]
    lines = [{"ID": "9", "QuestLineID": "100", "QuestID": "1"}]
    pois = [{"ID": "1", "QuestID": "2", "MapID": "1", "UiMapID": "2482"}]
    t = build_quests(quests, lines, pois).to_pylist()
    assert [r["id"] for r in t] == [1, 2]
    assert t[0]["quest_line_id"] == 100 and t[0]["map_id"] is None
    assert t[1]["quest_line_id"] is None and t[1]["ui_map_id"] == 2482


def test_provenance_and_url():
    prov = make_provenance("1.60.1.71037", "2026-11-20T00:00:00Z", ["Item", "ItemSparse"])
    assert prov["credit"] == CREDIT and prov["build"] == "1.60.1.71037"
    assert (
        table_url("Item", "1.60.1.71037") == "https://wago.tools/db2/Item/csv?version=1.60.1.71037"
    )


def test_build_all_stamps_sources(tmp_path):
    import csv
    import json

    def write(name, rows):
        keys = sorted({k for r in rows for k in r})
        with (tmp_path / f"{name}.csv").open("w", newline="") as f:
            w = csv.DictWriter(f, fieldnames=keys)
            w.writeheader()
            w.writerows(rows)

    write("ItemSparse", SPARSE)
    write("Item", ITEMS)
    write("QuestV2", [{"ID": "1", "UniqueBitFlag": "1", "UiQuestDetailsThemeID": "0"}])
    write("QuestLineXQuest", [{"ID": "1", "QuestLineID": "2", "QuestID": "1"}])
    write("QuestPOIBlob", [{"ID": "1", "QuestID": "1", "MapID": "1", "UiMapID": "3"}])
    prov = make_provenance("1.60.1.71037", "2026-11-20T00:00:00Z", ["Item"])
    (tmp_path / "provenance.json").write_text(json.dumps(prov))
    for table in build_all(tmp_path).values():
        (src,) = read_sources(table)
        assert src.version == "1.60.1.71037"
        assert src.retrieved == "2026-11-20T00:00:00Z"
        assert "Operator ruling" in src.license
