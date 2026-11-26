package tables

import "strings"

// ApplyRouteLearnBounds lowers an estimated learn skill that the guides' routes contradict.
// A route step that makes a recipe from skill N on shows the recipe can be learned by N, so an
// estimate above that is too high: it becomes the lowest N across every step (main route or
// alternative) that resolves to the recipe. Only rows whose learn_skill_source is "estimate"
// change, and they keep that source: a trainer's, an item's or the client's own figure is
// measured, and a disagreement with a guide there is the guide's to explain. Steps are matched
// as BuildProfessionRoutes matches them (profession and name, a shared name told apart by
// the yellow skill). It returns the recipes it lowered.
func ApplyRouteLearnBounds(recipes []Recipe, c CuratedRoutes) []Recipe {
	byRecipe := map[routeKey][]Recipe{}
	for _, r := range recipes {
		if r.Name != nil {
			k := routeKey{strings.ToLower(r.SkillLineName), strings.ToLower(*r.Name)}
			byRecipe[k] = append(byRecipe[k], r)
		}
	}
	first := map[int32]int{} // spell id -> lowest skill_from of a step that makes it
	for _, p := range c.professionKey {
		for _, s := range c.Professions[p].Steps {
			if s.Kind != "craft" {
				continue
			}
			spell := pickRecipe(byRecipe[routeKey{strings.ToLower(p), strings.ToLower(s.Recipe)}], s.SkillTo)
			if spell == nil {
				continue
			}
			if cur, ok := first[*spell]; !ok || s.SkillFrom < cur {
				first[*spell] = s.SkillFrom
			}
		}
	}
	var lowered []Recipe
	for i := range recipes {
		r := &recipes[i]
		from, ok := first[r.SpellID]
		if !ok || r.LearnSkill == nil || r.LearnSkillSource == nil || *r.LearnSkillSource != LearnSrcEstimate || int(*r.LearnSkill) <= from {
			continue
		}
		v := int16(from)
		r.LearnSkill = &v
		lowered = append(lowered, *r)
	}
	return lowered
}
