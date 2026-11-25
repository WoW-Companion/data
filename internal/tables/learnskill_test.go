package tables

import "testing"

func i16(v int16) *int16 { return &v }

func TestEstimateLearnClamps(t *testing.T) {
	tests := []struct {
		name         string
		yellow, grey *int16
		k            float64
		want         int16
		wantOK       bool
	}{
		{"yellow minus span", i16(230), i16(235), 1, 225, true},
		{"floor of one", i16(15), i16(55), 1, 1, true},
		{"never above yellow", i16(50), i16(40), 1, 50, true},
		{"no grey means yellow", i16(50), nil, 1, 50, true},
		{"expert floor", i16(240), i16(400), 1, 200, true},
		{"journeyman floor", i16(100), i16(300), 1, 50, true},
		{"no yellow", nil, i16(50), 1, 0, false},
		{"half span rounds", i16(100), i16(115), 0.5, 93, true},
	}
	for _, c := range tests {
		got, ok := EstimateLearn(c.yellow, c.grey, c.k)
		if ok != c.wantOK || got != c.want {
			t.Errorf("%s: got %d %v want %d %v", c.name, got, ok, c.want, c.wantOK)
		}
	}
}

func TestApplyLearnSkillsSourceOrder(t *testing.T) {
	recipes := []Recipe{
		{SpellID: 1, SkillLineName: "Alchemy", LearnSkill: i16(1), YellowSkill: i16(80), GreySkill: i16(100)},  // trainer wins over item
		{SpellID: 2, SkillLineName: "Alchemy", LearnSkill: i16(1), YellowSkill: i16(100), GreySkill: i16(120)}, // item (lowest)
		{SpellID: 3, SkillLineName: "Cooking", LearnSkill: i16(75), YellowSkill: i16(40), GreySkill: i16(55)},  // client, above yellow
		{SpellID: 4, SkillLineName: "Cooking", LearnSkill: i16(1), YellowSkill: i16(90), GreySkill: i16(100)},  // estimate
		{SpellID: 5, SkillLineName: "Cooking"},                                           // nothing
		{SpellID: 6, SkillLineName: "Cooking", LearnSkill: i16(1), YellowSkill: i16(30)}, // item without skill: estimate
	}
	items := []RecipeItem{
		{TeachesSpellID: 1, RequiredSkill: i16(50)},
		{TeachesSpellID: 2, RequiredSkill: i16(100)},
		{TeachesSpellID: 2, RequiredSkill: i16(80)},
		{TeachesSpellID: 6},
	}
	trainers := []TrainerSpell{
		{SpellID: func() *int32 { v := int32(1); return &v }(), RequiredSkill: i16(70)},
		{SpellID: func() *int32 { v := int32(1); return &v }(), RequiredSkill: i16(65)},
		{SpellID: nil, RequiredSkill: i16(5)},
	}
	st := ApplyLearnSkills(recipes, items, trainers)
	want := []struct {
		skill int16
		src   string
	}{{65, "trainer"}, {80, "recipe_item"}, {75, "client"}, {80, "estimate"}, {0, ""}, {30, "estimate"}}
	for i, w := range want {
		r := recipes[i]
		if w.src == "" {
			if r.LearnSkill != nil || r.LearnSkillSource != nil {
				t.Errorf("recipe %d: want null, got %v %v", r.SpellID, r.LearnSkill, r.LearnSkillSource)
			}
			continue
		}
		if r.LearnSkill == nil || *r.LearnSkill != w.skill || *r.LearnSkillSource != w.src {
			t.Errorf("recipe %d: got %v %v want %d %s", r.SpellID, r.LearnSkill, r.LearnSkillSource, w.skill, w.src)
		}
	}
	if len(st.AboveYellow) != 1 || st.AboveYellow[0].SpellID != 3 {
		t.Errorf("above yellow: %+v", st.AboveYellow)
	}
	if st.BySource["Cooking"]["estimate"] != 2 || st.BySource["Cooking"]["none"] != 1 || st.BySource["Alchemy"]["trainer"] != 1 {
		t.Errorf("counts: %+v", st.BySource)
	}
}

func TestApplyLearnSkillsFitsMultiplier(t *testing.T) {
	// Known rows all learn at yellow - span, so k=1 fits with zero error.
	recipes := []Recipe{
		{SpellID: 1, LearnSkill: i16(1), YellowSkill: i16(100), GreySkill: i16(120)},
		{SpellID: 2, LearnSkill: i16(1), YellowSkill: i16(60), GreySkill: i16(70)},
		{SpellID: 3, LearnSkill: i16(1), YellowSkill: i16(90), GreySkill: i16(110)},
	}
	items := []RecipeItem{{TeachesSpellID: 1, RequiredSkill: i16(80)}, {TeachesSpellID: 2, RequiredSkill: i16(50)}}
	st := ApplyLearnSkills(recipes, items, nil)
	if st.K != 1 || st.Known != 2 || st.MeanErr != 0 || st.P90Err != 0 {
		t.Errorf("fit: %+v", st)
	}
	if *recipes[2].LearnSkill != 70 || *recipes[2].LearnSkillSource != "estimate" {
		t.Errorf("estimate: %d", *recipes[2].LearnSkill)
	}
}
