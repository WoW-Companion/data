package tables

import (
	"slices"
)

// TalentTree is one row of the talent_trees table: one group of one trait tree. The
// client tables carry no name for a group or a tree, so GroupName is null; the class
// is null for a tree no skill line ties to a class. GroupOrder counts the tree's groups
// by ascending group id from 0. RequiredPoints is the points that must be spent before
// the group opens, null where the client lists no such condition.
type TalentTree struct {
	TreeID         int32   `parquet:"tree_id"`
	ClassID        *int8   `parquet:"class_id,optional"`
	ClassName      *string `parquet:"class_name,optional"`
	GroupID        int32   `parquet:"group_id"`
	GroupName      *string `parquet:"group_name,optional"`
	GroupOrder     int16   `parquet:"group_order"`
	RequiredPoints *int16  `parquet:"required_points,optional"`
}

// TalentNode is one row of the talent_nodes table. A node sits in several groups. Spell,
// name and max rank come from the node's first entry (by index); they are null for a node
// with no entry. PrerequisiteNodeIDs are the nodes with a TraitEdge to this one.
type TalentNode struct {
	NodeID              int32   `parquet:"node_id"`
	TreeID              int32   `parquet:"tree_id"`
	GroupIDs            []int32 `parquet:"group_ids,list"`
	SpellID             *int32  `parquet:"spell_id,optional"`
	Name                *string `parquet:"name,optional"`
	MaxRank             *int16  `parquet:"max_rank,optional"`
	PrerequisiteNodeIDs []int32 `parquet:"prerequisite_node_ids,list"`
}

// TalentInput is the client tables the talent builders read.
type TalentInput struct {
	Groups, GroupNodes, Conds, SkillTrees, SkillClasses, Classes []Row
	Nodes, NodeEntries, Entries, Definitions, Edges, SpellNames  []Row
}

func sortRowsByID(rows []Row) []Row {
	out := slices.Clone(rows)
	slices.SortStableFunc(out, func(a, b Row) int { return a.int("ID", 0) - b.int("ID", 0) })
	return out
}

func sortedUnique(v []int32) []int32 {
	out := slices.Clone(v)
	slices.Sort(out)
	out = slices.Compact(out)
	if out == nil {
		out = []int32{}
	}
	return out
}

// BuildTalentTrees makes one row per trait node group. A tree's class is the class of
// the single bit in the ClassMask of the skill line the client ties to the tree.
func BuildTalentTrees(in TalentInput) []TalentTree {
	classByBit := map[int]Row{}
	for _, c := range in.Classes {
		classByBit[1<<(c.int("ID", 0)-1)] = c
	}
	maskBySkill := map[int]int{}
	for _, r := range in.SkillClasses {
		maskBySkill[r.int("SkillID", 0)] = r.int("ClassMask", 0)
	}
	classOfTree := map[int]Row{}
	for _, r := range in.SkillTrees {
		if c, ok := classByBit[maskBySkill[r.int("SkillLineID", 0)]]; ok {
			classOfTree[r.int("TraitTreeID", 0)] = c
		}
	}
	required := map[int]int{}
	for _, c := range in.Conds {
		if g := c.int("TraitNodeGroupID", 0); g != 0 && c.int("SpentAmountRequired", 0) > 0 {
			required[g] = c.int("SpentAmountRequired", 0)
		}
	}
	order := map[int]int{}
	var out []TalentTree
	for _, g := range sortRowsByID(in.Groups) {
		tree, id := g.int("TraitTreeID", 0), g.int("ID", 0)
		t := TalentTree{TreeID: int32(tree), GroupID: int32(id), GroupOrder: int16(order[tree])}
		order[tree]++
		if c, ok := classOfTree[tree]; ok {
			cid, name := int8(c.int("ID", 0)), c["Name_lang"]
			t.ClassID, t.ClassName = &cid, &name
		}
		if n, ok := required[id]; ok {
			v := int16(n)
			t.RequiredPoints = &v
		}
		out = append(out, t)
	}
	slices.SortStableFunc(out, func(a, b TalentTree) int {
		if a.TreeID != b.TreeID {
			return int(a.TreeID - b.TreeID)
		}
		return int(a.GroupID - b.GroupID)
	})
	return out
}

// BuildTalentNodes makes one row per trait node, sorted by node id.
func BuildTalentNodes(in TalentInput) []TalentNode {
	groups := map[int][]int32{}
	for _, r := range in.GroupNodes {
		n := r.int("TraitNodeID", 0)
		groups[n] = append(groups[n], int32(r.int("TraitNodeGroupID", 0)))
	}
	entryOf := map[int]int{} // node -> first entry id by _Index
	firstIdx := map[int]int{}
	for _, r := range in.NodeEntries {
		n, idx := r.int("TraitNodeID", 0), r.int("_Index", 0)
		if i, seen := firstIdx[n]; !seen || idx < i {
			firstIdx[n], entryOf[n] = idx, r.int("TraitNodeEntryID", 0)
		}
	}
	entries := map[int]Row{}
	for _, r := range in.Entries {
		entries[r.int("ID", 0)] = r
	}
	defs := map[int]Row{}
	for _, r := range in.Definitions {
		defs[r.int("ID", 0)] = r
	}
	spellNames := map[int]string{}
	for _, r := range in.SpellNames {
		spellNames[r.int("ID", 0)] = r["Name_lang"]
	}
	prereq := map[int][]int32{}
	for _, r := range in.Edges {
		to := r.int("RightTraitNodeID", 0)
		prereq[to] = append(prereq[to], int32(r.int("LeftTraitNodeID", 0)))
	}
	out := make([]TalentNode, 0, len(in.Nodes))
	for _, r := range sortRowsByID(in.Nodes) {
		id := r.int("ID", 0)
		n := TalentNode{NodeID: int32(id), TreeID: int32(r.int("TraitTreeID", 0)), GroupIDs: sortedUnique(groups[id]), PrerequisiteNodeIDs: sortedUnique(prereq[id])}
		if e, ok := entries[entryOf[id]]; ok {
			mr := int16(e.int("MaxRanks", 0))
			n.MaxRank = &mr
			if d, ok := defs[e.int("TraitDefinitionID", 0)]; ok {
				if sp, ok := d.optInt("SpellID"); ok && sp != 0 {
					n.SpellID = ptr(sp)
				}
				name := d["OverrideName_lang"]
				if name == "" && n.SpellID != nil {
					name = spellNames[int(*n.SpellID)]
				}
				if name != "" {
					n.Name = &name
				}
			}
		}
		out = append(out, n)
	}
	return out
}
