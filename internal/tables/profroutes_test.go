package tables

import (
	"path/filepath"
	"strings"
	"testing"
)

var routeProfessions = []string{"Alchemy", "Blacksmithing", "Cooking", "Enchanting", "Engineering", "First Aid", "Fishing",
	"Herbalism", "Leatherworking", "Mining", "Skinning", "Tailoring"}

func loadRealRoutes(t *testing.T) CuratedRoutes {
	t.Helper()
	c, err := LoadCuratedRoutes(filepath.Join("..", "..", "curated", RoutesFile))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// mainStepAt returns the main-route step whose range holds skill (from <= skill < to), and its index.
func mainStepAt(steps []CuratedStep, skill int) (CuratedStep, int) {
	for i, s := range steps {
		if s.Option == 0 && s.SkillFrom <= skill && skill < s.SkillTo {
			return s, i
		}
	}
	return CuratedStep{}, -1
}

func TestCuratedRoutesEnchantingFifty(t *testing.T) {
	steps := loadRealRoutes(t).Professions["Enchanting"].Steps
	s, i := mainStepAt(steps, 51)
	if s.Recipe != "Enchant Chest - Inferior Stamina" || len(s.Materials) != 1 || s.Materials[0].Item != "Strange Dust" || s.Materials[0].Count != 1 {
		t.Fatalf("step at 51: %+v", s)
	}
	if s.SkillFrom != 20 || s.SkillTo != 70 {
		t.Errorf("range %d-%d, want 20-70", s.SkillFrom, s.SkillTo)
	}
	next, _ := mainStepAt(steps[i+1:], 70)
	if next.Recipe != "Enchant Chest - Minor Absorption" || next.Tier != "Journeyman" {
		t.Errorf("step after: %+v", next)
	}
}

func TestCuratedRoutesCoverage(t *testing.T) {
	c := loadRealRoutes(t)
	for _, p := range routeProfessions {
		prof, ok := c.Professions[p]
		if !ok {
			t.Errorf("%s: no route", p)
			continue
		}
		if gaps, top := RouteCoverage(prof.Steps); len(gaps) > 0 || top != 300 {
			t.Errorf("%s main route: gaps %v, reaches %d", p, gaps, top)
		}
		if !strings.HasPrefix(prof.SourceURL, "https://www.wow-professions.com/forever/") {
			t.Errorf("%s: source %q", p, prof.SourceURL)
		}
	}
}

func TestParseCuratedRoutesRejects(t *testing.T) {
	ok := `{"fetched_at":"2026-11-26T00:00:00Z","professions":{"Mining":{"source_url":"u","steps":[%s]}}}`
	step := `{"step":1,"option":0,"kind":"craft","tier":"Apprentice","skill_from":1,"skill_to":65,"recipe":"Smelt Copper","crafts":60,"materials":[{"item":"Copper Ore","count":1}],"learned_from":"trainer"}`
	for name, tc := range map[string]struct{ step, want string }{
		"good":         {step, ""},
		"bad kind":     {strings.Replace(step, `"craft"`, `"brew"`, 1), "unknown kind"},
		"bad range":    {strings.Replace(step, `"skill_to":65`, `"skill_to":1`, 1), "bad skill range"},
		"no crafts":    {strings.Replace(step, `"crafts":60,`, ``, 1), "needs a recipe and crafts"},
		"zero count":   {strings.Replace(step, `"count":1`, `"count":0`, 1), "bad material"},
		"bad step no.": {strings.Replace(step, `"step":1`, `"step":3`, 1), "numbered 1..n"},
		"alt on main":  {strings.Replace(step, `"learned_from"`, `"alternative_of":1,"learned_from"`, 1), "alternative_of belongs"},
	} {
		_, err := ParseCuratedRoutes("t.json", []byte(strings.Replace(ok, "%s", tc.step, 1)))
		if (tc.want == "") != (err == nil) || (err != nil && !strings.Contains(err.Error(), tc.want)) {
			t.Errorf("%s: err %v, want %q", name, err, tc.want)
		}
	}
}

func TestBuildProfessionRoutesResolves(t *testing.T) {
	raw := `{"fetched_at":"2026-11-26T00:00:00Z","professions":{"Enchanting":{"source_url":"u",
	 "tiers":[{"tier":"Expert","skill_required":125,"min_level":20,"cost_copper":5000,"how":"trainer","trainer":"Hgarth","place":"Stonetalon Mountains","faction":"Horde"}],
	 "steps":[
	  {"step":1,"kind":"craft","tier":"Apprentice","skill_from":20,"skill_to":70,"recipe":"Enchant Chest - Inferior Stamina","crafts":50,"materials":[{"item":"Strange Dust","count":1}],"learned_from":"trainer"},
	  {"step":2,"option":1,"kind":"craft","tier":"Apprentice","skill_from":20,"skill_to":70,"recipe":"Unknown Enchant","crafts":50,"materials":[{"item":"Nothing Dust","count":1}],"learned_from":"vendor","learned_by":[{"name":"Kithas","place":"Orgrimmar"}],"alternative_of":1}]}}}`
	c, err := ParseCuratedRoutes("t.json", []byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	name := "Enchant Chest - Inferior Stamina"
	recipes := []Recipe{{SpellID: 7, SkillLineName: "Enchanting", Name: &name}}
	items := []Item{{ID: 10940, Name: "Strange Dust"}}
	npcs := []Npc{{ID: 5, Name: "Kithas"}, {ID: 6, Name: "Hgarth"}}
	routes, tiers, rep := BuildProfessionRoutes(c, recipes, items, npcs, nil)
	if len(routes) != 2 || routes[0].RecipeSpellID == nil || *routes[0].RecipeSpellID != 7 || routes[0].Materials[0].ItemID == nil || *routes[0].Materials[0].ItemID != 10940 {
		t.Fatalf("routes: %+v", routes)
	}
	if routes[1].RecipeSpellID != nil || routes[1].Materials[0].ItemID != nil || routes[1].RecipeName == nil || len(routes[1].LearnedNpcIDs) != 1 || *routes[1].AlternativeOf != 1 {
		t.Errorf("unresolved row: %+v", routes[1])
	}
	if rep.UnresolvedRecipes != 1 || rep.UnresolvedItems != 1 || len(tiers) != 1 || tiers[0].NpcID == nil || *tiers[0].NpcID != 6 {
		t.Errorf("report %+v tiers %+v", rep, tiers)
	}
}

func TestPickRecipeByYellowSkill(t *testing.T) {
	y := func(n int16) *int16 { return &n }
	rs := []Recipe{{SpellID: 1, YellowSkill: y(185)}, {SpellID: 2, YellowSkill: y(220)}}
	if got := pickRecipe(rs, 200); got == nil || *got != 1 {
		t.Errorf("got %v, want 1", got)
	}
	if got := pickRecipe(rs, 300); got != nil {
		t.Errorf("two fit: got %v, want nil", *got)
	}
}

func TestMissingRoutesFileIsEmpty(t *testing.T) {
	c, err := LoadCuratedRoutes(filepath.Join(t.TempDir(), "none.json"))
	if err != nil || len(c.Professions) != 0 {
		t.Errorf("%v %v", c, err)
	}
}

func TestRouteTablesWriteParquet(t *testing.T) {
	routes, tiers, _ := BuildProfessionRoutes(loadRealRoutes(t), nil, nil, nil, nil)
	src := CuratedSource("b", "t")
	dir := t.TempDir()
	r, err := writeTable(dir, "profession_routes.parquet", routes, Layout(nil, []string{"profession", "step"}, src))
	if err != nil || r.Rows != int64(len(routes)) || len(routes) < 200 {
		t.Fatalf("routes: %+v %v (%d rows)", r, err, len(routes))
	}
	r, err = writeTable(dir, "profession_tiers.parquet", tiers, Layout(nil, []string{"profession", "tier"}, src))
	if err != nil || r.Rows != int64(len(tiers)) || len(tiers) < 30 {
		t.Fatalf("tiers: %+v %v (%d rows)", r, err, len(tiers))
	}
}
