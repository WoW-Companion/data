# curated/

Hand-maintained YAML that people review in pull requests: stat weights per spec
and level band, talent builds and profession routes. Each file records where its
numbers came from. 
## talents/

`talents/<class>/<spec>.yaml` holds an old, invented and unverified talent build for one spec (to be replaced by the Icy Veins WoW Forever leveling builds, the reference the Operator chose; none has been read yet): level
bands (10-19, 20-29, 30-39), each with the points gained in that band in the order to spend
them, and a short rationale written for this project. Every build is an advisor default, not
authoritative. A step is `{node: <talent_nodes.node_id>, name: <node name>, points: <n>}`;
`group_id` names the spec's talent group (the tables carry no group names). The Priest
(Discipline, Holy, Shadow Magic), Rogue (Assassination, Combat, Subtlety), Warrior (Arms, Fury,
Protection), Paladin (Holy, Protection, Retribution) and Hunter (Beast Mastery, Marksmanship,
Survival), Druid (Balance, Feral Combat, Restoration), Mage (Arcane, Fire, Frost), Shaman
(Elemental, Enhancement, Restoration) and Warlock (Affliction, Demonology, Destruction) specs are
covered, which is every class in `talent_trees`.

`make dataset` validates every file against `talent_trees` and `talent_nodes` and fails on any
problem, then publishes the `talent_builds` table (source "curated (unverified)").
`go test ./internal/tables` runs the same check when `dist/cache/<build>` exists. The checks:
the group belongs to the class's tree; each node is in that group and carries the given name;
points never exceed a node's max rank; the points spent through a band never exceed the points
at its top level; a node with prerequisites takes its first point only once each prerequisite
is at max rank.

Assumptions: the fetched tables hold no currency rules, so a character has one talent point
per level from level 10 (level 39 has 30). Talent rows ("tiers") that open after N points spent
are not checked: the tables give no per-node requirement, only groups with a points condition,
so the builds avoid nodes that look deeper than the points spent.

## instances.yaml

`instances.yaml` holds the typical level range and the entrance zone (UiMap id and name) of the
classic dungeons and raids, one flow map per `Map.ID`, with a `source` on every row. It is an
advisor default, not authoritative. `make dataset` checks every row against `instances` (the map
exists and carries that name) and `UiMap` (the zone id carries that name), and fails on a
mismatch. The build fills `instances.min_level` and `max_level` from it only where the client's
content tuning holds none (`level_source` says which), and uses the entrance zone unless
Everloot's corroborated observation names one (`entrance_zone_source`). Maps without a row
(cities, scenarios, season-specific maps) keep null levels.

## professions.yaml

`professions.yaml` holds profession notes in our own words from conventional knowledge: each profession and whether it is primary or secondary, the rank tiers with skill caps and the character level each needs, pairings, feeders, special cases and rules of thumb (the skinning rule). It is an advisor default, not authoritative. Sections hold one flow map per line; every field of a section is required. `make dataset` checks the file and every profession name against the client's profession skill lines, and publishes it as `profession_notes`.

## profession_routes.json

Each profession's 1-300 leveling route and its rank trainers, as facts read from the WoW Forever guides on wow-professions.com (the reference the Operator chose for profession advice). `scripts/extract_routes.py <dir of saved pages>` writes it; the notes in it are in our own words. The build resolves names to ids, prints what it cannot resolve and any gap in a main route, and publishes the `profession_routes` and `profession_tiers` tables (docs/sources.md). Edit the JSON by hand only for a fix the extractor cannot make; re-run the extractor when the guides change.

## talent_priorities.json

Optional. Per spec, the Icy Veins WoW Forever guide's ranked key leveling talents (`node_id`, `name`) with a one-line `reason` in our own words, plus `source_url` and a top-level `fetched_at`. Each node is checked against `talent_nodes` at `make dataset`; a missing file publishes an empty `talent_priorities` table.
