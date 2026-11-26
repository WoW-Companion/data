package tables

import (
	"slices"
	"strings"
)

// WeaponSkill is one row of weapon_skills: a class and a weapon skill line, with whether the
// class can learn that skill and whether it starts knowing it.
type WeaponSkill struct {
	ClassID      int32  `parquet:"class_id"`
	ClassName    string `parquet:"class_name,dict"`
	SkillLineID  int32  `parquet:"skill_line_id"`
	SkillName    string `parquet:"skill_name,dict"`
	Learnable    bool   `parquet:"learnable"`
	DefaultKnown bool   `parquet:"default_known"`
	MinLevel     *int16 `parquet:"min_level,optional"`
}

// weaponSkillCategory is the SkillLine.CategoryID of weapon skill lines.
const weaponSkillCategory = 6

// notTaughtWeapon lists category-6 lines that are not weapon skills a Weapon Master teaches:
// Defense, Dual Wield, Unarmed (everyone has it) and Feral Combat (a druid form skill).
var notTaughtWeapon = map[int]bool{95: true, 118: true, 162: true, 3014: true}

// weaponSkillNames overrides the client's line name where it is ambiguous: the client calls
// the one-handed lines "Swords", "Axes" and "Maces".
var weaponSkillNames = map[int]string{43: "One-Handed Swords", 44: "One-Handed Axes", 54: "One-Handed Maces"}

// WeaponSkillInput is the client tables BuildWeaponSkills reads.
type WeaponSkillInput struct {
	SkillLines, SkillClasses, Classes []Row
}

// BuildWeaponSkills makes one row per class and weapon skill line (category 6, less the
// notTaughtWeapon lines). A SkillRaceClassInfo row whose ClassMask holds the class (-1 is every
// class) makes the skill learnable; one with Availability 1 makes it known from character
// creation, for at least one race (the rows carry race masks, which are not published).
// MinLevel is the lowest MinLevel across the class's rows, null when 0. Rows are sorted by
// class then skill line.
func BuildWeaponSkills(in WeaponSkillInput) []WeaponSkill {
	lines := map[int]string{}
	for _, l := range in.SkillLines {
		id := l.int("ID", 0)
		if l.int("CategoryID", 0) != weaponSkillCategory || notTaughtWeapon[id] {
			continue
		}
		name := l["DisplayName_lang"]
		if n, ok := weaponSkillNames[id]; ok {
			name = n
		}
		lines[id] = name
	}
	type key struct{ class, line int }
	type state struct {
		learn, known bool
		min          int
	}
	st := map[key]*state{}
	for _, r := range in.SkillClasses {
		line := r.int("SkillID", 0)
		if _, ok := lines[line]; !ok {
			continue
		}
		mask := r.int("ClassMask", 0)
		for _, c := range in.Classes {
			id := c.int("ID", 0)
			if id <= 0 || (mask >= 0 && mask&(1<<(id-1)) == 0) {
				continue
			}
			lvl := r.int("MinLevel", 0)
			s := st[key{id, line}]
			if s == nil {
				s = &state{min: lvl}
				st[key{id, line}] = s
			}
			s.learn = true
			s.known = s.known || r.int("Availability", 0) == 1
			s.min = min(s.min, lvl)
		}
	}
	out := []WeaponSkill{}
	for _, c := range in.Classes {
		cid := c.int("ID", 0)
		if cid <= 0 {
			continue
		}
		for line, name := range lines {
			w := WeaponSkill{ClassID: int32(cid), ClassName: c["Name_lang"], SkillLineID: int32(line), SkillName: name}
			if s := st[key{cid, line}]; s != nil {
				w.Learnable, w.DefaultKnown = s.learn, s.known
				if s.min > 0 {
					w.MinLevel = ptr16(s.min)
				}
			}
			out = append(out, w)
		}
	}
	slices.SortFunc(out, func(a, b WeaponSkill) int {
		if a.ClassID != b.ClassID {
			return int(a.ClassID - b.ClassID)
		}
		return int(a.SkillLineID - b.SkillLineID)
	})
	return out
}

// WeaponMaster is one row of weapon_masters: a Weapon Master NPC and what they teach.
// Skills, CostCopper and MinLevel are aligned lists, one entry per skill taught.
type WeaponMaster struct {
	NpcID      int32   `parquet:"npc_id"`
	Name       string  `parquet:"name"`
	Faction    string  `parquet:"faction,dict"`
	City       string  `parquet:"city,dict"`
	Place      *string `parquet:"place,optional,dict"`
	ZoneID     int32   `parquet:"zone_id"`
	X          float64 `parquet:"x"`
	Y          float64 `parquet:"y"`
	Skills     []int32 `parquet:"skills,list"`
	CostCopper []int32 `parquet:"cost_copper,list"`
	MinLevel   []int16 `parquet:"min_level,list"`
	Source     string  `parquet:"source,dict"`
}

// Weapon skill lines the masters teach, by client SkillLine id.
const (
	skSwords1, skAxes1, skBows, skGuns, skMaces1 = 43, 44, 45, 46, 54
	skSwords2, skStaves, skMaces2, skAxes2       = 55, 136, 160, 172
	skDaggers, skThrown, skCrossbows, skPolearms = 173, 176, 226, 229
	skFist                                       = 473
)

const (
	weaponTrainCost   = 1000  // 10 silver
	polearmTrainCost  = 10000 // 1 gold
	polearmTrainLevel = 20
	masterSource      = "curated: Wowhead Classic NPC pages (teaches lists, locations); costs and levels from the Forever rules"
)

type masterDef struct {
	id          int32
	name, side  string
	city, place string
	zone        int32 // UiMap id of the city
	x, y        float64
	skills      []int32
}

// curatedMasters are the Weapon Masters, facts only. The client tables carry no trainer
// offers (docs/sources.md), so the NPC ids, locations and the skills each teaches come from
// the public Wowhead Classic NPC pages (the "Teaches" list of npc=<id>), and the place names
// Valley of Honor, Lower Rise and War Quarter from the Forever guides. Cost is 10 silver a
// skill and Polearms 1 gold at level 20. Place is empty where no source gave one.
var curatedMasters = []masterDef{
	{2704, "Hanashi", "Horde", "Orgrimmar", "Valley of Honor", 1637, 81.4, 19.4, []int32{skAxes1, skAxes2, skStaves, skBows, skThrown}},
	{11868, "Sayoc", "Horde", "Orgrimmar", "Valley of Honor", 1637, 81.4, 19.4, []int32{skAxes1, skAxes2, skBows, skDaggers, skThrown, skFist}},
	{11869, "Ansekhwa", "Horde", "Thunder Bluff", "Lower Rise", 1638, 40.4, 62.4, []int32{skMaces1, skMaces2, skStaves, skGuns}},
	{11870, "Archibald", "Horde", "Undercity", "War Quarter", 1497, 56.4, 33.2, []int32{skPolearms, skSwords1, skSwords2, skDaggers, skCrossbows}},
	{11865, "Buliwyf Stonehand", "Alliance", "Ironforge", "", 1537, 61.4, 89.2, []int32{skAxes1, skAxes2, skMaces1, skMaces2, skGuns, skFist}},
	{11866, "Ilyenia Moonfire", "Alliance", "Darnassus", "", 1657, 57.2, 45.4, []int32{skStaves, skBows, skDaggers, skThrown, skFist}},
	{11867, "Woo Ping", "Alliance", "Stormwind City", "", 1519, 57, 57.4, []int32{skPolearms, skSwords1, skSwords2, skStaves, skDaggers, skCrossbows}},
}

// BuildWeaponMasters returns the curated masters as rows sorted by NPC id, with each skill's
// cost and minimum level. A master teaching a skill that is not in skills is dropped, and so
// is one whose id is in npcs under another name, so a bad id never ships.
func BuildWeaponMasters(skills []WeaponSkill, npcs []Npc) []WeaponMaster {
	valid := map[int32]bool{}
	for _, s := range skills {
		valid[s.SkillLineID] = true
	}
	known := map[int32]string{}
	for _, n := range npcs {
		known[n.ID] = n.Name
	}
	out := []WeaponMaster{}
	for _, d := range curatedMasters {
		if n, ok := known[d.id]; ok && !strings.EqualFold(n, d.name) {
			continue
		}
		m := WeaponMaster{NpcID: d.id, Name: d.name, Faction: d.side, City: d.city, ZoneID: d.zone, X: d.x, Y: d.y, Source: masterSource}
		if d.place != "" {
			p := d.place
			m.Place = &p
		}
		ok := true
		for _, sk := range d.skills {
			if !valid[sk] {
				ok = false
				break
			}
			cost, lvl := int32(weaponTrainCost), int16(1)
			if sk == skPolearms {
				cost, lvl = polearmTrainCost, polearmTrainLevel
			}
			m.Skills = append(m.Skills, sk)
			m.CostCopper = append(m.CostCopper, cost)
			m.MinLevel = append(m.MinLevel, lvl)
		}
		if ok {
			out = append(out, m)
		}
	}
	slices.SortFunc(out, func(a, b WeaponMaster) int { return int(a.NpcID - b.NpcID) })
	return out
}
