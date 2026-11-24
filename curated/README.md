# curated/

Hand-maintained YAML that people review in pull requests: stat weights per spec
and level band, talent builds and profession routes. Each file records where its
numbers came from. 
## talents/

`talents/<class>/<spec>.yaml` holds an advisor-default talent build for one spec: level
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
problem, then publishes the `talent_builds` table (source "curated (advisor defaults)").
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
