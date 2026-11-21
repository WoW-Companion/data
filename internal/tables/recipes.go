package tables

import (
	"slices"
	"strings"

	"github.com/WoW-Companion/data/internal/everloot"
)

// Recipe is one row of the recipes table: a profession spell. YellowSkill and GreySkill are
// the skill levels at which the recipe turns yellow and grey, from the skill line ability's
// trivial rank low and high; learn skill is its minimum rank. A zero in the client means
// none and is stored as null. The client stores no green threshold, so there is no column.
type Recipe struct {
	SpellID       int32   `parquet:"spell_id"`
	SkillLineID   int32   `parquet:"skill_line_id"`
	SkillLineName string  `parquet:"skill_line_name"`
	Name          *string `parquet:"name,optional"`
	CreatesItemID *int32  `parquet:"creates_item_id,optional"`
	LearnSkill    *int16  `parquet:"learn_skill,optional"`
	YellowSkill   *int16  `parquet:"yellow_skill,optional"`
	GreySkill     *int16  `parquet:"grey_skill,optional"`
}

// RecipeReagent is one row of the recipe_reagents table.
type RecipeReagent struct {
	SpellID       int32 `parquet:"spell_id"`
	ReagentItemID int32 `parquet:"reagent_item_id"`
	Count         int16 `parquet:"count"`
}

// RecipeInput is the client tables the recipe builders read.
type RecipeInput struct {
	SkillLines, Abilities, Reagents, Effects, SpellNames []Row
}

const reagentSlots = 8

// Spell effects that make an item: CreateItem and CreateRandomItem.
func isCreateItemEffect(e int) bool { return e == 24 || e == 157 }

// isProfessionLine is a profession or secondary-profession skill line (category 11 or 9)
// that is not a test line. Category 9 also holds riding and racial lines; they have no
// recipe spells, and a recipe needs reagents or a created item anyway.
func isProfessionLine(r Row) bool {
	c := r.int("CategoryID", 0)
	return (c == 11 || c == 9) && !strings.Contains(r["DisplayName_lang"], "[DNT]")
}

func recipeReagents(in RecipeInput) map[int][]RecipeReagent {
	out := map[int][]RecipeReagent{}
	for _, r := range in.Reagents {
		spell := r.int("SpellID", 0)
		for i := 0; i < reagentSlots; i++ {
			s := string(rune('0' + i))
			item, n := r.int("Reagent_"+s, 0), r.int("ReagentCount_"+s, 0)
			if item > 0 && n > 0 {
				out[spell] = append(out[spell], RecipeReagent{SpellID: int32(spell), ReagentItemID: int32(item), Count: int16(n)})
			}
		}
	}
	return out
}

func createdItems(effects []Row) map[int]int {
	idx := map[int]int{}
	out := map[int]int{}
	for _, r := range effects {
		item := r.int("EffectItemType", 0)
		if !isCreateItemEffect(r.int("Effect", 0)) || item == 0 {
			continue
		}
		spell, ei := r.int("SpellID", 0), r.int("EffectIndex", 0)
		if i, seen := idx[spell]; !seen || ei < i {
			idx[spell], out[spell] = ei, item
		}
	}
	return out
}

func optSkill(v int) *int16 {
	if v <= 0 {
		return nil
	}
	x := int16(v)
	return &x
}

// BuildRecipes makes one row per (spell, profession skill line) skill line ability whose
// spell has reagents or creates an item, sorted by skill line then spell.
func BuildRecipes(in RecipeInput) []Recipe {
	lines := map[int]string{}
	for _, r := range in.SkillLines {
		if isProfessionLine(r) {
			lines[r.int("ID", 0)] = r["DisplayName_lang"]
		}
	}
	names := map[int]string{}
	for _, r := range in.SpellNames {
		names[r.int("ID", 0)] = r["Name_lang"]
	}
	reagents, creates := recipeReagents(in), createdItems(in.Effects)
	out := []Recipe{}
	seen := map[[2]int]bool{}
	for _, a := range in.Abilities {
		line, spell := a.int("SkillLine", 0), a.int("Spell", 0)
		lineName, ok := lines[line]
		if !ok || seen[[2]int{line, spell}] {
			continue
		}
		item, makes := creates[spell]
		if len(reagents[spell]) == 0 && !makes {
			continue
		}
		seen[[2]int{line, spell}] = true
		rc := Recipe{
			SpellID: int32(spell), SkillLineID: int32(line), SkillLineName: lineName,
			LearnSkill:  optSkill(a.int("MinSkillLineRank", 0)),
			YellowSkill: optSkill(a.int("TrivialSkillLineRankLow", 0)),
			GreySkill:   optSkill(a.int("TrivialSkillLineRankHigh", 0)),
		}
		if n := names[spell]; n != "" {
			rc.Name = &n
		}
		if makes {
			rc.CreatesItemID = ptr(item)
		}
		out = append(out, rc)
	}
	slices.SortFunc(out, func(a, b Recipe) int {
		if a.SkillLineID != b.SkillLineID {
			return int(a.SkillLineID - b.SkillLineID)
		}
		return int(a.SpellID - b.SpellID)
	})
	return out
}

// BuildRecipeReagents makes one row per reagent of each recipe spell, merging a reagent
// listed twice, sorted by spell then item.
func BuildRecipeReagents(recipes []Recipe, in RecipeInput) []RecipeReagent {
	byRecipe := recipeReagents(in)
	spells := map[int32]bool{}
	for _, r := range recipes {
		spells[r.SpellID] = true
	}
	out := []RecipeReagent{}
	for spell := range spells {
		merged := map[int32]int16{}
		for _, r := range byRecipe[int(spell)] {
			merged[r.ReagentItemID] += r.Count
		}
		for item, n := range merged {
			out = append(out, RecipeReagent{SpellID: spell, ReagentItemID: item, Count: n})
		}
	}
	slices.SortFunc(out, func(a, b RecipeReagent) int {
		if a.SpellID != b.SpellID {
			return int(a.SpellID - b.SpellID)
		}
		return int(a.ReagentItemID - b.ReagentItemID)
	})
	return out
}

// BuildCraftedSources makes one `crafted` loot_sources row per recipe with a created item:
// source_id is the recipe spell, profession the skill line name and learn_skill the recipe's
// minimum rank. A spell listed under two skill lines yields one row (the lower line id).
// Counts and rates stay zero: there is no drop chance. recipes must be in BuildRecipes order.
func BuildCraftedSources(recipes []Recipe) []everloot.LootSource {
	out := []everloot.LootSource{}
	seen := map[[2]int32]bool{}
	for _, r := range recipes {
		if r.CreatesItemID == nil || *r.CreatesItemID <= 0 {
			continue
		}
		k := [2]int32{*r.CreatesItemID, r.SpellID}
		if seen[k] {
			continue
		}
		seen[k] = true
		name := r.SkillLineName
		out = append(out, everloot.LootSource{
			ItemID: *r.CreatesItemID, SourceKind: everloot.KindCrafted, SourceID: r.SpellID,
			Profession: &name, LearnSkill: r.LearnSkill,
		})
	}
	return out
}

// MergeLootSources appends the crafted rows to the Everloot rows and sorts the union. Rows
// already in base keep their values.
func MergeLootSources(base, crafted []everloot.LootSource) []everloot.LootSource {
	out := append(slices.Clone(base), crafted...)
	everloot.SortLootSources(out)
	return out
}
