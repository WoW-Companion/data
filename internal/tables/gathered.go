package tables

import (
	"slices"

	"github.com/WoW-Companion/data/internal/everloot"
)

// gameObjectChest is the GameObjects TypeID of an openable object; its PropValue_0 is the
// Lock row that says what opens it.
const gameObjectChest = 3

// lockBySkill is the Lock.Type value of a slot that is a skill requirement (LockType in
// _Index_n, the rank in Skill_n).
const lockBySkill = 2

// Node is a gathering node the client tables identify: a game object whose lock needs
// Herbalism or Mining.
type Node struct {
	ID            int32
	Name          string
	Profession    string
	SkillRequired int16
}

// GatherInput holds the client tables a node is read from.
type GatherInput struct {
	GameObjects []Row
	Locks       []Row
	LockTypes   []Row
}

// gatherProfessions are the LockType names that make a node a gathering node.
var gatherProfessions = []string{"Herbalism", "Mining"}

// BuildNodes returns the gathering nodes by game object id. A node is a chest-type object
// whose lock has a skill slot of a LockType named Herbalism or Mining; the first such slot
// gives the skill. Nothing else (name, item, zone) makes an object a node.
func BuildNodes(in GatherInput) map[int32]Node {
	types := map[int]string{}
	for _, t := range in.LockTypes {
		if n := t["Name_lang"]; slices.Contains(gatherProfessions, n) {
			types[t.int("ID", 0)] = n
		}
	}
	locks := map[int]Row{}
	for _, l := range in.Locks {
		locks[l.int("ID", 0)] = l
	}
	out := map[int32]Node{}
	for _, g := range in.GameObjects {
		lock, ok := locks[g.int("PropValue_0", 0)]
		if !ok || g.int("TypeID", -1) != gameObjectChest {
			continue
		}
		for i := 0; i < 8; i++ {
			n := string(rune('0' + i))
			prof, ok := types[lock.int("_Index_"+n, 0)]
			if !ok || lock.int("Type_"+n, 0) != lockBySkill {
				continue
			}
			id := g.int("ID", 0)
			out[int32(id)] = Node{ID: int32(id), Name: g["Name_lang"], Profession: prof, SkillRequired: int16(lock.int("Skill_"+n, 0))}
			break
		}
	}
	return out
}

// ApplyGathered turns the object rows whose source is a known node into `gathered` rows,
// keeping Everloot's counts and rates, and returns how many it changed. Object rows for any
// other object stay as they are.
func ApplyGathered(rows []everloot.LootSource, nodes map[int32]Node) int {
	n := 0
	for i := range rows {
		node, ok := nodes[rows[i].SourceID]
		if !ok || rows[i].SourceKind != everloot.KindObject {
			continue
		}
		rows[i].SourceKind = everloot.KindGathered
		id, name, prof, skill := node.ID, node.Name, node.Profession, node.SkillRequired
		rows[i].NodeID, rows[i].Profession, rows[i].SkillRequired = &id, &prof, &skill
		if name != "" {
			rows[i].NodeName = &name
		}
		n++
	}
	everloot.SortLootSources(rows)
	return n
}
