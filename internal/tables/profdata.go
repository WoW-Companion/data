package tables

import (
	"slices"

	"github.com/WoW-Companion/data/internal/everloot"
)

// Trainer is one row of trainers: a trainer and a profession it teaches, corroborated by
// Everloot's trainer windows. Position and faction are null: a trainer window carries
// neither.
type Trainer struct {
	NpcID      int32    `parquet:"npc_id"`
	Name       *string  `parquet:"name,optional"`
	Profession string   `parquet:"profession,dict"`
	ZoneID     *int32   `parquet:"zone_id,optional"`
	ZoneName   *string  `parquet:"zone_name,optional,dict"`
	X          *float64 `parquet:"x,optional"`
	Y          *float64 `parquet:"y,optional"`
	Faction    *string  `parquet:"faction,optional,dict"`
	Recipes    int32    `parquet:"recipes"`
	Reporters  int32    `parquet:"reporters"`
}

// TrainerSpell is one row of trainer_spells: one recipe a trainer teaches. SpellID is null
// when the recipe's name matches no recipe (or several) of that profession.
type TrainerSpell struct {
	NpcID         int32  `parquet:"npc_id"`
	SpellID       *int32 `parquet:"spell_id,optional"`
	Recipe        string `parquet:"recipe_name"`
	Profession    string `parquet:"profession,dict"`
	CostCopper    *int64 `parquet:"cost_copper,optional"`
	RequiredSkill *int16 `parquet:"required_skill,optional"`
	Reporters     int32  `parquet:"reporters"`
}

func uiNames(uiMaps []Row) map[int32]string {
	out := map[int32]string{}
	for _, m := range uiMaps {
		if n := m["Name_lang"]; n != "" {
			out[int32(m.int("ID", 0))] = n
		}
	}
	return out
}

// BuildTrainers makes trainer_spells and trainers from corroborated offers. A recipe name
// links to a spell when exactly one recipe of the offer's profession carries it. A trainer's
// zone is Everloot's located_in zone for the creature; its name comes from npcs.
func BuildTrainers(offers []everloot.TrainerOffer, recipes []Recipe, npcs []Npc, zones map[int32]everloot.NpcZone, uiMaps []Row, professions []string) ([]Trainer, []TrainerSpell) {
	type pn struct{ prof, name string }
	bySpell := map[pn][]int32{}
	for _, r := range recipes {
		if r.Name != nil {
			k := pn{r.SkillLineName, *r.Name}
			if !slices.Contains(bySpell[k], r.SpellID) {
				bySpell[k] = append(bySpell[k], r.SpellID)
			}
		}
	}
	name := map[int32]string{}
	for _, n := range npcs {
		name[n.ID] = n.Name
	}
	ui := uiNames(uiMaps)
	spells := []TrainerSpell{}
	type tk struct {
		npc  int32
		prof string
	}
	agg := map[tk]*Trainer{}
	for _, o := range offers {
		if len(professions) > 0 && !slices.Contains(professions, o.Profession) {
			continue // a profession named in an install's own language
		}
		ts := TrainerSpell{NpcID: o.NpcID, Recipe: o.Recipe, Profession: o.Profession, CostCopper: o.CostCopper, RequiredSkill: o.RequiredSkill, Reporters: o.Reporters}
		if ids := bySpell[pn{o.Profession, o.Recipe}]; len(ids) == 1 {
			ts.SpellID = &ids[0]
		}
		spells = append(spells, ts)
		t := agg[tk{o.NpcID, o.Profession}]
		if t == nil {
			t = &Trainer{NpcID: o.NpcID, Profession: o.Profession}
			if n, ok := name[o.NpcID]; ok && n != "" {
				t.Name = &n
			}
			if z, ok := zones[o.NpcID]; ok {
				t.ZoneID = &z.ZoneID
				if n, ok := ui[z.ZoneID]; ok {
					t.ZoneName = &n
				}
			}
			agg[tk{o.NpcID, o.Profession}] = t
		}
		t.Recipes++
		t.Reporters = max(t.Reporters, o.Reporters)
	}
	trainers := []Trainer{}
	for _, t := range agg {
		trainers = append(trainers, *t)
	}
	slices.SortFunc(trainers, func(a, b Trainer) int {
		if a.NpcID != b.NpcID {
			return int(a.NpcID - b.NpcID)
		}
		return everlootCmp(a.Profession, b.Profession)
	})
	return trainers, spells
}

func everlootCmp(a, b string) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// TrainedSpells is the set of recipe spells some trainer offer links to.
func TrainedSpells(ts []TrainerSpell) map[int32]bool {
	out := map[int32]bool{}
	for _, t := range ts {
		if t.SpellID != nil {
			out[*t.SpellID] = true
		}
	}
	return out
}

// GatheringNode is one row of gathering_nodes: a herb or mining node in a zone, from
// Everloot's loot windows. Profession and skill come from the client's lock when it names
// the node, else the profession from Everloot's gather kind (ore: Mining, herb:
// Herbalism) and the skill is null.
type GatheringNode struct {
	NodeID        int32   `parquet:"node_id"`
	Name          *string `parquet:"name,optional,dict"`
	Profession    string  `parquet:"profession,dict"`
	SkillRequired *int16  `parquet:"skill_required,optional"`
	ZoneID        int32   `parquet:"zone_id"`
	ZoneName      *string `parquet:"zone_name,optional,dict"`
	Windows       int32   `parquet:"windows"`
	Reporters     int32   `parquet:"reporters"`
}

// GatheringYield is one row of gathering_yields: an item a node gave.
type GatheringYield struct {
	NodeID    int32 `parquet:"node_id"`
	ItemID    int32 `parquet:"item_id"`
	Windows   int32 `parquet:"windows"`
	Reporters int32 `parquet:"reporters"`
	MinCount  int32 `parquet:"min_count"`
	MaxCount  int32 `parquet:"max_count"`
}

// BuildGathering turns node observations into rows. Only herb and mining nodes are kept: a
// node the client names (nodes) or Everloot labels ore or herb. Yields follow their nodes.
//
// Skill and profession come from the client lock first, then from curatedNodeSkills by the
// node's exact name. Everloot's drop rates (drops: object or gathered rows) add a yield for
// an item a kept node dropped that the loot windows did not give at two installs; such a
// yield carries the rate row's kills as windows and reporters, and 0 for both counts, which
// the rate rows do not hold.
func BuildGathering(obs []everloot.NodeObs, yields []everloot.NodeYield, nodes map[int32]Node, uiMaps []Row, drops []everloot.LootSource) ([]GatheringNode, []GatheringYield) {
	ui := uiNames(uiMaps)
	rows := []GatheringNode{}
	kept := map[int32]bool{}
	for _, o := range obs {
		g := GatheringNode{NodeID: o.ObjectID, ZoneID: o.ZoneID, Windows: o.Windows, Reporters: o.Reporters}
		if n, ok := nodes[o.ObjectID]; ok {
			g.Profession = n.Profession
			if n.SkillRequired > 0 {
				g.SkillRequired = &n.SkillRequired
			}
			if n.Name != "" {
				nm := n.Name
				g.Name = &nm
			}
		}
		name := o.Name
		if g.Name != nil {
			name = *g.Name
		}
		cur, curOK := curatedNodeSkills[name]
		if g.Profession == "" && curOK {
			g.Profession = cur.Profession
		}
		if g.Profession == "" {
			switch o.GatherKind {
			case everloot.GatherOre:
				g.Profession = "Mining"
			case everloot.GatherHerb:
				g.Profession = "Herbalism"
			default:
				continue
			}
		}
		if g.Name == nil && o.Name != "" {
			nm := o.Name
			g.Name = &nm
		}
		if g.SkillRequired == nil && curOK && cur.Profession == g.Profession {
			sk := cur.Skill
			g.SkillRequired = &sk
		}
		if n, ok := ui[o.ZoneID]; ok {
			g.ZoneName = &n
		}
		kept[o.ObjectID] = true
		rows = append(rows, g)
	}
	ys := []GatheringYield{}
	for _, y := range yields {
		if kept[y.ObjectID] {
			ys = append(ys, GatheringYield{NodeID: y.ObjectID, ItemID: y.ItemID, Windows: y.Windows, Reporters: y.Reporters, MinCount: y.MinCount, MaxCount: y.MaxCount})
		}
	}
	have := map[[2]int32]bool{}
	for _, y := range ys {
		have[[2]int32{y.NodeID, y.ItemID}] = true
	}
	for _, d := range drops {
		if (d.SourceKind != everloot.KindObject && d.SourceKind != everloot.KindGathered) || !kept[d.SourceID] || d.Drops <= 0 || have[[2]int32{d.SourceID, d.ItemID}] {
			continue
		}
		have[[2]int32{d.SourceID, d.ItemID}] = true
		ys = append(ys, GatheringYield{NodeID: d.SourceID, ItemID: d.ItemID, Windows: d.Kills, Reporters: d.Reporters})
	}
	slices.SortFunc(ys, func(a, b GatheringYield) int {
		if a.NodeID != b.NodeID {
			return int(a.NodeID - b.NodeID)
		}
		return int(a.ItemID - b.ItemID)
	})
	return rows, ys
}

// FishingCatch is one row of fishing_catches.
type FishingCatch struct {
	ZoneID    int32   `parquet:"zone_id"`
	ZoneName  *string `parquet:"zone_name,optional,dict"`
	ItemID    int32   `parquet:"item_id"`
	Casts     int32   `parquet:"casts"`
	Catches   int32   `parquet:"catches"`
	Reporters int32   `parquet:"reporters"`
	Rate      float64 `parquet:"rate"`
}

// BuildFishingCatches names the zones of the observed catches.
func BuildFishingCatches(c []everloot.FishCatch, uiMaps []Row) []FishingCatch {
	ui := uiNames(uiMaps)
	out := []FishingCatch{}
	for _, f := range c {
		row := FishingCatch{ZoneID: f.ZoneID, ItemID: f.ItemID, Casts: f.Casts, Catches: f.Catches, Reporters: f.Reporters, Rate: f.Rate}
		if n, ok := ui[f.ZoneID]; ok {
			row.ZoneName = &n
		}
		out = append(out, row)
	}
	return out
}
