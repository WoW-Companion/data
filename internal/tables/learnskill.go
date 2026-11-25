package tables

import (
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/WoW-Companion/data/internal/everloot"
)

// Values of recipes.learn_skill_source.
const (
	LearnSrcTrainer    = "trainer"
	LearnSrcRecipeItem = "recipe_item"
	LearnSrcClient     = "client"
	LearnSrcEstimate   = "estimate"
)

// tierCaps are the rank caps of Apprentice, Journeyman, Expert and Artisan. They only
// sanity-check the estimate and set its floor; they never supply a learn skill.
var tierCaps = []int{75, 150, 225, 300}

// tierFloor is the lowest skill a recipe whose yellow threshold passes a tier cap can be
// learned at: the skill that tier's trainer asks for (Journeyman 50, Expert 125, Artisan
// 200), 25 under the previous cap. 1 for an Apprentice-range recipe.
func tierFloor(yellow int) int {
	floor := 1
	for _, c := range tierCaps[:3] {
		if yellow > c {
			floor = c - 25
		}
	}
	return floor
}

// estimateKs are the candidate multipliers of the yellow-to-grey span the fit chooses from.
// 1 is the documented default "yellow - (grey - yellow)".
var estimateKs = []float64{0, 0.5, 1, 1.5, 2}

// EstimateLearn is the estimate rule: yellow - k*(grey-yellow), rounded, raised to the tier
// floor, then clamped to between 1 and yellow (the clamp to yellow wins). ok is false when
// the recipe has no yellow threshold.
func EstimateLearn(yellow, grey *int16, k float64) (int16, bool) {
	if yellow == nil {
		return 0, false
	}
	y := int(*yellow)
	span := 0
	if grey != nil && int(*grey) > y {
		span = int(*grey) - y
	}
	v := int(math.Round(float64(y) - k*float64(span)))
	v = max(v, tierFloor(y), 1)
	v = min(v, y)
	return int16(v), true
}

// LearnStats is what ApplyLearnSkills reports for the build log and the mission report.
type LearnStats struct {
	// BySource counts recipes per profession and source; recipes with no learn skill are
	// counted under "none".
	BySource map[string]map[string]int
	// K is the fitted multiplier; Known is the number of recipes it was fitted on and
	// MeanErr, MedianErr and P90Err the absolute error of the estimate on them.
	K                          float64
	Known                      int
	MeanErr, MedianErr, P90Err float64
	// Candidates is the mean absolute error of each candidate multiplier.
	Candidates map[float64]float64
	// AboveYellow lists recipes whose learn skill is greater than their yellow threshold.
	AboveYellow []Recipe
}

// ApplyLearnSkills sets learn_skill and learn_skill_source on every recipe. recipes still
// hold the client's MinSkillLineRank in LearnSkill. Order: a trainer's required skill (the
// lowest across offers), a recipe item's required skill (the lowest across items), the
// client minimum when above 1, else an estimate fitted on the recipes with a known source.
// A recipe with none of these and no yellow threshold keeps a null learn skill and source.
func ApplyLearnSkills(recipes []Recipe, items []RecipeItem, trainers []TrainerSpell) LearnStats {
	byTrainer, byItem := map[int32]int16{}, map[int32]int16{}
	lowest := func(m map[int32]int16, spell int32, v *int16) {
		if v == nil || *v <= 0 {
			return
		}
		if cur, ok := m[spell]; !ok || *v < cur {
			m[spell] = *v
		}
	}
	for _, t := range trainers {
		if t.SpellID != nil {
			lowest(byTrainer, *t.SpellID, t.RequiredSkill)
		}
	}
	for _, it := range items {
		lowest(byItem, it.TeachesSpellID, it.RequiredSkill)
	}
	var pending []int
	for i := range recipes {
		r := &recipes[i]
		client := r.LearnSkill
		r.LearnSkill, r.LearnSkillSource = nil, nil
		set := func(v int16, src string) { r.LearnSkill, r.LearnSkillSource = &v, &src }
		if v, ok := byTrainer[r.SpellID]; ok {
			set(v, LearnSrcTrainer)
		} else if v, ok := byItem[r.SpellID]; ok {
			set(v, LearnSrcRecipeItem)
		} else if client != nil && *client > 1 {
			set(*client, LearnSrcClient)
		} else {
			pending = append(pending, i)
		}
	}
	st := LearnStats{BySource: map[string]map[string]int{}, Candidates: map[float64]float64{}}
	var known []*Recipe
	for i := range recipes {
		if r := &recipes[i]; r.LearnSkill != nil && r.YellowSkill != nil {
			known = append(known, r)
		}
	}
	st.Known = len(known)
	errs := func(k float64) []float64 {
		out := make([]float64, 0, len(known))
		for _, r := range known {
			e, _ := EstimateLearn(r.YellowSkill, r.GreySkill, k)
			out = append(out, math.Abs(float64(e)-float64(*r.LearnSkill)))
		}
		return out
	}
	st.K, st.MeanErr = 1, math.NaN()
	for _, k := range estimateKs {
		m := mean(errs(k))
		st.Candidates[k] = m
		if math.IsNaN(st.MeanErr) || m < st.MeanErr {
			st.K, st.MeanErr = k, m
		}
	}
	if len(known) > 0 {
		e := errs(st.K)
		slices.Sort(e)
		st.MedianErr, st.P90Err = e[len(e)/2], e[min(len(e)-1, int(math.Ceil(0.9*float64(len(e))))-1)]
	} else {
		st.MeanErr = 0
	}
	for _, i := range pending {
		r := &recipes[i]
		if v, ok := EstimateLearn(r.YellowSkill, r.GreySkill, st.K); ok {
			src := LearnSrcEstimate
			r.LearnSkill, r.LearnSkillSource = &v, &src
		}
	}
	for _, r := range recipes {
		src := "none"
		if r.LearnSkillSource != nil {
			src = *r.LearnSkillSource
		}
		if st.BySource[r.SkillLineName] == nil {
			st.BySource[r.SkillLineName] = map[string]int{}
		}
		st.BySource[r.SkillLineName][src]++
		if r.LearnSkill != nil && r.YellowSkill != nil && *r.LearnSkill > *r.YellowSkill {
			st.AboveYellow = append(st.AboveYellow, r)
		}
	}
	return st
}

// Report is the build log's account of the stats: per-profession source counts, the error of
// each candidate multiplier and the recipes whose learn skill is above their yellow one.
func (st LearnStats) Report() string {
	var b strings.Builder
	profs := make([]string, 0, len(st.BySource))
	for p := range st.BySource {
		profs = append(profs, p)
	}
	slices.Sort(profs)
	for _, p := range profs {
		c := st.BySource[p]
		fmt.Fprintf(&b, "learn skill source %s: trainer %d, recipe_item %d, client %d, estimate %d, none %d\n", p, c[LearnSrcTrainer], c[LearnSrcRecipeItem], c[LearnSrcClient], c[LearnSrcEstimate], c["none"])
	}
	for _, k := range estimateKs {
		fmt.Fprintf(&b, "learn skill estimate candidate k=%g: mean abs error %.2f\n", k, st.Candidates[k])
	}
	for _, r := range st.AboveYellow {
		name := ""
		if r.Name != nil {
			name = *r.Name
		}
		fmt.Fprintf(&b, "learn above yellow: %s spell %d %q learn %d (%s) yellow %d grey %v\n", r.SkillLineName, r.SpellID, name, *r.LearnSkill, *r.LearnSkillSource, *r.YellowSkill, derefOr(r.GreySkill))
	}
	return b.String()
}

func derefOr(p *int16) any {
	if p == nil {
		return "null"
	}
	return *p
}

func mean(x []float64) float64 {
	if len(x) == 0 {
		return math.NaN()
	}
	s := 0.0
	for _, v := range x {
		s += v
	}
	return s / float64(len(x))
}

// SyncCraftedLearnSkill copies each recipe's final learn skill into the crafted
// loot_sources rows, which were built from the client minimum before the sources were
// resolved. A spell listed under two lines takes the lower line's value, as the rows do.
func SyncCraftedLearnSkill(rows []everloot.LootSource, recipes []Recipe) {
	bySpell := map[int32]*int16{}
	for _, r := range recipes { // sorted by line: the first wins
		if _, ok := bySpell[r.SpellID]; !ok {
			bySpell[r.SpellID] = r.LearnSkill
		}
	}
	for i := range rows {
		if rows[i].SourceKind == everloot.KindCrafted {
			rows[i].LearnSkill = bySpell[rows[i].SourceID]
		}
	}
}
