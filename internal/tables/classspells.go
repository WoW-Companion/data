package tables

import (
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// classSkillCategory is the SkillLine.CategoryID of class skill lines (Arms, Fury, Holy ...).
const classSkillCategory = 7

// ClassSpell is one row of the class_spells table: a class ability rank on a class skill line.
// Trained is always null: the client tables do not say whether a trainer teaches a spell.
type ClassSpell struct {
	ClassID           int32   `parquet:"class_id"`
	SpellID           int32   `parquet:"spell_id"`
	Name              string  `parquet:"name"`
	Rank              *int16  `parquet:"rank,optional"`
	Subtext           *string `parquet:"subtext,optional"`
	Level             int16   `parquet:"level"`
	SkillLineID       int32   `parquet:"skill_line_id"`
	SkillLineName     string  `parquet:"skill_line_name"`
	SupersededSpellID *int32  `parquet:"superseded_spell_id,optional"`
	Trained           *bool   `parquet:"trained,optional"`
}

// ClassSpellInput is the client tables BuildClassSpells reads.
type ClassSpellInput struct {
	SkillLines, Abilities, SkillClasses, Classes, SpellNames, Spells, SpellLevels []Row
}

// notClassLine is a category-7 line that belongs to a pet or a companion, not to the class's own
// abilities (Pet - Wolf, Beast Training, Explorer Imp).
func notClassLine(name string) bool {
	return strings.HasPrefix(name, "Pet") || strings.HasPrefix(name, "Beast Training") || strings.HasPrefix(name, "Explorer")
}

var rankRe = regexp.MustCompile(`^Rank (\d+)$`)

// classMasks returns the class ids set in a ChrClasses bit mask (class id n is bit n-1).
func classMasks(mask int, known map[int]bool) []int {
	var out []int
	for id := range known {
		if mask&(1<<(id-1)) != 0 {
			out = append(out, id)
		}
	}
	slices.Sort(out)
	return out
}

// BuildClassSpells makes one row per (class, skill line, spell) skill line ability on a class
// skill line (CategoryID 7). Pet lines are left out by name; a line must belong to exactly one class
// by its SkillRaceClassInfo masks, which drops Mounts, Companions, Runes and Engraving (shared by
// every class); an ability's own ClassMask, when set, narrows it further.
// Talents are not skill line abilities, professions and racials are other categories.
// Level is the spell's SpellLevels SpellLevel (BaseLevel when that is zero) from the
// difficulty-0 row; an ability with neither is left out. Rank comes from a "Rank N" subtext.
// Rows are sorted by class, level, spell, skill line.
func BuildClassSpells(in ClassSpellInput) []ClassSpell {
	classes := map[int]bool{}
	for _, c := range in.Classes {
		if id := c.int("ID", 0); id > 0 {
			classes[id] = true
		}
	}
	lines := map[int]string{}
	for _, l := range in.SkillLines {
		if l.int("CategoryID", 0) == classSkillCategory && !notClassLine(l["DisplayName_lang"]) {
			lines[l.int("ID", 0)] = l["DisplayName_lang"]
		}
	}
	lineMask := map[int]int{}
	for _, r := range in.SkillClasses {
		lineMask[r.int("SkillID", 0)] |= r.int("ClassMask", 0)
	}
	for id := range lines {
		if m := lineMask[id]; m <= 0 || m&(m-1) != 0 { // not exactly one class: Mounts, Runes, Companions ...
			delete(lines, id)
		}
	}
	names := map[int]string{}
	for _, r := range in.SpellNames {
		names[r.int("ID", 0)] = r["Name_lang"]
	}
	subtext := map[int]string{}
	for _, r := range in.Spells {
		subtext[r.int("ID", 0)] = r["NameSubtext_lang"]
	}
	level := map[int]int{}
	for _, r := range in.SpellLevels {
		if r.int("DifficultyID", 0) != 0 {
			continue
		}
		l := r.int("SpellLevel", 0)
		if l == 0 {
			l = r.int("BaseLevel", 0)
		}
		if id := r.int("SpellID", 0); l > 0 && (level[id] == 0 || l < level[id]) {
			level[id] = l
		}
	}
	out := []ClassSpell{}
	seen := map[[3]int]bool{}
	for _, a := range in.Abilities {
		line, spell := a.int("SkillLine", 0), a.int("Spell", 0)
		lineName, ok := lines[line]
		lvl := level[spell]
		if !ok || lvl == 0 || names[spell] == "" {
			continue
		}
		mask := lineMask[line]
		if m := a.int("ClassMask", 0); m > 0 {
			mask &= m
		}
		for _, class := range classMasks(mask, classes) {
			if seen[[3]int{class, line, spell}] {
				continue
			}
			seen[[3]int{class, line, spell}] = true
			row := ClassSpell{
				ClassID: int32(class), SpellID: int32(spell), Name: names[spell], Level: int16(lvl),
				SkillLineID: int32(line), SkillLineName: lineName,
			}
			if s := subtext[spell]; s != "" {
				row.Subtext = &s
				if m := rankRe.FindStringSubmatch(s); m != nil {
					n, _ := strconv.Atoi(m[1])
					r := int16(n)
					row.Rank = &r
				}
			}
			if sup := a.int("SupercedesSpell", 0); sup > 0 {
				row.SupersededSpellID = ptr(sup)
			}
			out = append(out, row)
		}
	}
	slices.SortFunc(out, func(a, b ClassSpell) int {
		for _, d := range []int{int(a.ClassID - b.ClassID), int(a.Level - b.Level), int(a.SpellID - b.SpellID), int(a.SkillLineID - b.SkillLineID)} {
			if d != 0 {
				return d
			}
		}
		return 0
	})
	return out
}
