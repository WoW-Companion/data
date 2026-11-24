package tables

import (
	"slices"
)

// spellEffectLearnSpell is the SpellEffect.Effect value of "learn spell".
const spellEffectLearnSpell = 36

// ItemEffect.TriggerType values: 0 on use (the use spell may have a LearnSpell effect) and 6
// learn spell (ItemEffect.SpellID is the spell taught). The client's LearnSpell effects leave
// EffectTriggerSpell at 0 for recipe items, so the second form carries almost every recipe.
const (
	triggerOnUse      = 0
	triggerLearnSpell = 6
)

// Values of recipes.learned_from.
const (
	LearnedTrainer = "trainer"
	LearnedItem    = "item"
	LearnedBoth    = "both"
)

// RecipeItem is one row of recipe_items: an item (a Pattern, Plans, Recipe, Formula,
// Schematic or Manual) whose use spell teaches a recipe spell.
type RecipeItem struct {
	ItemID         int32  `parquet:"item_id"`
	TeachesSpellID int32  `parquet:"teaches_spell_id"`
	Profession     string `parquet:"profession,dict"`
	// RequiredSkill is ItemSparse.RequiredSkillRank (the skill needed to use the item); null
	// when the client stores zero.
	RequiredSkill *int16 `parquet:"required_skill,optional"`
}

// RecipeItemInput is the client tables the link is read from.
type RecipeItemInput struct {
	ItemEffects, ItemLinks, SpellEffects, Items []Row
}

// BuildRecipeItems links items to the recipe spells they teach: an item's ItemEffect spell
// (TriggerType 0, on use; ItemXItemEffect says which items carry the effect) has a LearnSpell SpellEffect whose EffectTriggerSpell is a spell
// in recipes. The profession is the recipe's skill line (the lowest line id when a spell is
// listed under two). Sorted by item and spell.
func BuildRecipeItems(recipes []Recipe, in RecipeItemInput) []RecipeItem {
	line := map[int32]string{}
	for _, r := range recipes { // recipes are sorted by skill line, so the first wins
		if _, ok := line[r.SpellID]; !ok {
			line[r.SpellID] = r.SkillLineName
		}
	}
	learns := map[int][]int{} // using-spell -> recipe spells it teaches
	for _, e := range in.SpellEffects {
		if e.int("Effect", 0) != spellEffectLearnSpell {
			continue
		}
		if t := e.int("EffectTriggerSpell", 0); t > 0 {
			if _, ok := line[int32(t)]; ok {
				learns[e.int("SpellID", 0)] = append(learns[e.int("SpellID", 0)], t)
			}
		}
	}
	skill := map[int]int{}
	for _, it := range in.Items {
		skill[it.int("ID", 0)] = it.int("RequiredSkillRank", 0)
	}
	parent := map[int][]int{} // ItemEffect id -> items that carry it
	for _, l := range in.ItemLinks {
		parent[l.int("ItemEffectID", 0)] = append(parent[l.int("ItemEffectID", 0)], l.int("ItemID", 0))
	}
	type key struct{ item, spell int }
	seen := map[key]bool{}
	out := []RecipeItem{}
	for _, e := range in.ItemEffects {
		var teaches []int
		switch e.int("TriggerType", -1) {
		case triggerOnUse:
			teaches = learns[e.int("SpellID", 0)]
		case triggerLearnSpell:
			if sp := e.int("SpellID", 0); sp > 0 {
				if _, ok := line[int32(sp)]; ok {
					teaches = []int{sp}
				}
			}
		}
		for _, sp := range teaches {
			for _, item := range parent[e.int("ID", 0)] {
				if item <= 0 || seen[key{item, sp}] {
					continue
				}
				seen[key{item, sp}] = true
				out = append(out, RecipeItem{ItemID: int32(item), TeachesSpellID: int32(sp), Profession: line[int32(sp)], RequiredSkill: optSkill(skill[item])})
			}
		}
	}
	slices.SortFunc(out, func(a, b RecipeItem) int {
		if a.ItemID != b.ItemID {
			return int(a.ItemID - b.ItemID)
		}
		return int(a.TeachesSpellID - b.TeachesSpellID)
	})
	return out
}

// ApplyLearnedFrom sets every recipe's learned_from: item when a recipe item teaches the
// spell, both when a corroborated trainer offer also does, trainer otherwise. It returns
// the counts of each.
func ApplyLearnedFrom(recipes []Recipe, items []RecipeItem, trained map[int32]bool) (trainer, item, both int) {
	byItem := map[int32]bool{}
	for _, r := range items {
		byItem[r.TeachesSpellID] = true
	}
	for i := range recipes {
		s := recipes[i].SpellID
		switch {
		case byItem[s] && trained[s]:
			recipes[i].LearnedFrom = LearnedBoth
			both++
		case byItem[s]:
			recipes[i].LearnedFrom = LearnedItem
			item++
		default:
			recipes[i].LearnedFrom = LearnedTrainer
			trainer++
		}
	}
	return
}
