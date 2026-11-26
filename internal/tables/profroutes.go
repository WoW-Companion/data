package tables

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"sort"
	"strings"
)

// RouteMaterial is one material of a route step: what one craft uses. ItemID is null when the
// name matched no item (or several) of the dataset. A Count of 0 marks something gathered along
// the way (a herb or ore the step picks) rather than spent.
type RouteMaterial struct {
	ItemID *int32 `parquet:"item_id,optional"`
	Name   string `parquet:"name"`
	Count  int16  `parquet:"count"`
}

// ProfessionRoute is one row of profession_routes: one step of a profession's leveling path.
type ProfessionRoute struct {
	Profession string `parquet:"profession,dict"`
	Step       int16  `parquet:"step"`
	// Option is 0 on the main route and 1, 2, ... on the alternatives the guide gives.
	Option        int8            `parquet:"option"`
	Kind          string          `parquet:"kind,dict"`
	Tier          string          `parquet:"tier,dict"`
	SkillFrom     int16           `parquet:"skill_from"`
	SkillTo       int16           `parquet:"skill_to"`
	RecipeSpellID *int32          `parquet:"recipe_spell_id,optional"`
	RecipeName    *string         `parquet:"recipe_name,optional"`
	Crafts        *int16          `parquet:"crafts,optional"`
	Materials     []RouteMaterial `parquet:"materials,list"`
	Where         []string        `parquet:"where,list"`
	LearnedFrom   string          `parquet:"learned_from,dict"`
	LearnedBy     []string        `parquet:"learned_by,list"`
	LearnedNpcIDs []int32         `parquet:"learned_npc_ids,list"`
	// AlternativeOf is the step this one replaces; null on the main route.
	AlternativeOf *int16  `parquet:"alternative_of,optional"`
	Notes         *string `parquet:"notes,optional"`
	SourceURL     string  `parquet:"source_url,dict"`
	FetchedAt     string  `parquet:"fetched_at,dict"`
}

// ProfessionTier is one row of profession_tiers: a rank of a profession (Journeyman, Expert,
// Artisan) and where it is learned: one row per trainer, or one row with no trainer where the
// guide names none.
type ProfessionTier struct {
	Profession    string  `parquet:"profession,dict"`
	Tier          string  `parquet:"tier,dict"`
	SkillRequired *int16  `parquet:"skill_required,optional"`
	MinLevel      *int16  `parquet:"min_level,optional"`
	CostCopper    *int64  `parquet:"cost_copper,optional"`
	How           string  `parquet:"how,dict"`
	TrainerName   *string `parquet:"trainer_name,optional"`
	NpcID         *int32  `parquet:"npc_id,optional"`
	Faction       *string `parquet:"faction,optional,dict"`
	Place         *string `parquet:"place,optional"`
	Note          *string `parquet:"note,optional"`
	SourceURL     string  `parquet:"source_url,dict"`
	FetchedAt     string  `parquet:"fetched_at,dict"`
}

// RoutesFile is the curated routes file's name, kept beside curated/professions.yaml.
const RoutesFile = "profession_routes.json"

// CuratedRoutes is the parsed curated/profession_routes.json: the facts read from the guides,
// by name. BuildProfessionRoutes resolves the names to dataset ids.
type CuratedRoutes struct {
	Source        string                      `json:"source"`
	FetchedAt     string                      `json:"fetched_at"`
	Professions   map[string]CuratedRouteProf `json:"professions"`
	professionKey []string
}

// CuratedRouteProf is one profession's section of the curated routes file.
type CuratedRouteProf struct {
	SourceURL string        `json:"source_url"`
	Tiers     []CuratedTier `json:"tiers"`
	Steps     []CuratedStep `json:"steps"`
}

// CuratedTier is one trainer-tier fact of the curated file.
type CuratedTier struct {
	Tier          string `json:"tier"`
	SkillRequired *int16 `json:"skill_required"`
	MinLevel      *int16 `json:"min_level"`
	CostCopper    *int64 `json:"cost_copper"`
	How           string `json:"how"`
	Trainer       string `json:"trainer"`
	Place         string `json:"place"`
	Faction       string `json:"faction"`
	Note          string `json:"note"`
}

// CuratedStep is one route step of the curated file.
type CuratedStep struct {
	Step          int               `json:"step"`
	Option        int               `json:"option"`
	Kind          string            `json:"kind"`
	Tier          string            `json:"tier"`
	SkillFrom     int               `json:"skill_from"`
	SkillTo       int               `json:"skill_to"`
	Recipe        string            `json:"recipe"`
	Crafts        *int              `json:"crafts"`
	Materials     []CuratedMaterial `json:"materials"`
	Where         []string          `json:"where"`
	LearnedFrom   string            `json:"learned_from"`
	LearnedBy     []CuratedVendor   `json:"learned_by"`
	AlternativeOf *int              `json:"alternative_of"`
	Notes         string            `json:"notes"`
}

// CuratedMaterial is one material of a curated step: an item name and a per-craft count.
type CuratedMaterial struct {
	Item  string `json:"item"`
	Count int    `json:"count"`
}

// CuratedVendor names an NPC the step's recipe is learned from, with where they stand.
type CuratedVendor struct {
	Name  string `json:"name"`
	Place string `json:"place"`
}

// LoadCuratedRoutes reads and checks the curated routes file. A missing file is no routes (the
// tables build empty); a malformed one is an error.
func LoadCuratedRoutes(path string) (CuratedRoutes, error) {
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return CuratedRoutes{}, nil
	}
	if err != nil {
		return CuratedRoutes{}, err
	}
	return ParseCuratedRoutes(path, raw)
}

var routeKinds = []string{"craft", "gather"}
var routeLearned = []string{"trainer", "vendor", "drop", "quest", "item"}

// ParseCuratedRoutes parses and checks the curated routes JSON: known kinds and sources, skill
// ranges inside 1-300, step numbers 1..n per profession, alternative_of pointing at a main-route
// step, per-craft counts of at least 1 on crafts.
func ParseCuratedRoutes(path string, raw []byte) (CuratedRoutes, error) {
	var c CuratedRoutes
	if err := json.Unmarshal(raw, &c); err != nil {
		return c, fmt.Errorf("%s: %w", path, err)
	}
	for p := range c.Professions {
		c.professionKey = append(c.professionKey, p)
	}
	sort.Strings(c.professionKey)
	if c.FetchedAt == "" || len(c.Professions) == 0 {
		return c, fmt.Errorf("%s: want fetched_at and at least one profession", path)
	}
	for _, p := range c.professionKey {
		prof := c.Professions[p]
		if prof.SourceURL == "" {
			return c, fmt.Errorf("%s: %s has no source_url", path, p)
		}
		for i, s := range prof.Steps {
			where := fmt.Sprintf("%s: %s step %d", path, p, s.Step)
			switch {
			case s.Step != i+1:
				return c, fmt.Errorf("%s: steps must be numbered 1..n in order (got %d at position %d)", where, s.Step, i+1)
			case !slices.Contains(routeKinds, s.Kind):
				return c, fmt.Errorf("%s: unknown kind %q", where, s.Kind)
			case !slices.Contains(routeLearned, s.LearnedFrom):
				return c, fmt.Errorf("%s: unknown learned_from %q", where, s.LearnedFrom)
			case s.SkillFrom < 1 || s.SkillTo > 300 || s.SkillFrom >= s.SkillTo:
				return c, fmt.Errorf("%s: bad skill range %d-%d", where, s.SkillFrom, s.SkillTo)
			case s.Kind == "craft" && (s.Recipe == "" || s.Crafts == nil || *s.Crafts < 1):
				return c, fmt.Errorf("%s: a craft step needs a recipe and crafts", where)
			case s.Option == 0 && s.AlternativeOf != nil, s.Option > 0 && s.AlternativeOf == nil:
				return c, fmt.Errorf("%s: alternative_of belongs on option steps only", where)
			case len(s.Notes) > 300:
				return c, fmt.Errorf("%s: notes are %d characters; keep them short", where, len(s.Notes))
			}
			if s.AlternativeOf != nil {
				a := *s.AlternativeOf
				if a < 1 || a > len(prof.Steps) || prof.Steps[a-1].Option != 0 {
					return c, fmt.Errorf("%s: alternative_of %d is not a main-route step", where, a)
				}
			}
			for _, m := range s.Materials {
				if m.Item == "" || m.Count < 0 || (s.Kind == "craft" && m.Count < 1) {
					return c, fmt.Errorf("%s: bad material %+v", where, m)
				}
			}
		}
	}
	return c, nil
}

// RouteCoverage lists the skill ranges a profession's main route leaves uncovered between 1
// and its last step's top, as "a-b" strings; the guides' routes should have none. Two main steps
// over the same range (a "make both" step) count once.
func RouteCoverage(steps []CuratedStep) (gaps []string, top int) {
	reach := 1
	for _, s := range steps {
		if s.Option != 0 {
			continue
		}
		if s.SkillFrom > reach {
			gaps = append(gaps, fmt.Sprintf("%d-%d", reach, s.SkillFrom))
		}
		reach = max(reach, s.SkillTo)
	}
	return gaps, reach
}

// RouteReport is what resolving the curated routes against the dataset left unresolved.
type RouteReport struct {
	Rows, UnresolvedRecipes, UnresolvedItems, UnresolvedTrainers int
	Messages                                                     []string
}

func (r RouteReport) String() string {
	return fmt.Sprintf("profession routes: %d steps, %d recipes and %d materials not found in the dataset, %d trainers without an npc id",
		r.Rows, r.UnresolvedRecipes, r.UnresolvedItems, r.UnresolvedTrainers)
}

type routeKey struct{ line, name string }

// pickRecipe returns the spell of the one recipe in rs, or, when several share the name (a
// later-era copy of a classic recipe), the one that turns yellow at or below the step's top
// skill, if that leaves exactly one; else nil.
func pickRecipe(rs []Recipe, skillTo int) *int32 {
	if len(rs) == 1 {
		return &rs[0].SpellID
	}
	var fit []Recipe
	for _, r := range rs {
		if r.YellowSkill != nil && int(*r.YellowSkill) <= skillTo {
			fit = append(fit, r)
		}
	}
	if len(fit) == 1 {
		return &fit[0].SpellID
	}
	return nil
}

// BuildProfessionRoutes resolves the curated routes against the dataset: a recipe by profession
// and name against recipes, a material by name against items, a trainer or vendor by name
// against trainers and npcs. A name that matches nothing, or more than one, stays null and is
// reported; the row is kept so the advisor can skip it. Rows are sorted by profession then step.
func BuildProfessionRoutes(c CuratedRoutes, recipes []Recipe, items []Item, npcs []Npc, trainers []Trainer) ([]ProfessionRoute, []ProfessionTier, RouteReport) {
	var rep RouteReport
	byRecipe := map[routeKey][]Recipe{}
	for _, r := range recipes {
		if r.Name != nil {
			k := routeKey{strings.ToLower(r.SkillLineName), strings.ToLower(*r.Name)}
			byRecipe[k] = append(byRecipe[k], r)
		}
	}
	byItem := map[string][]int32{}
	for _, it := range items {
		k := strings.ToLower(it.Name)
		byItem[k] = append(byItem[k], it.ID)
	}
	byNpc := map[string][]int32{}
	for _, n := range npcs {
		k := strings.ToLower(n.Name)
		byNpc[k] = append(byNpc[k], n.ID)
	}
	trainerIDs := map[routeKey][]int32{}
	for _, t := range trainers {
		if t.Name != nil {
			k := routeKey{strings.ToLower(t.Profession), strings.ToLower(*t.Name)}
			trainerIDs[k] = append(trainerIDs[k], t.NpcID)
		}
	}
	one := func(ids []int32) *int32 {
		if len(ids) == 1 {
			return &ids[0]
		}
		return nil
	}
	var routes []ProfessionRoute
	var tiers []ProfessionTier
	for _, p := range c.professionKey {
		prof := c.Professions[p]
		for _, s := range prof.Steps {
			row := ProfessionRoute{
				Profession: p, Step: int16(s.Step), Option: int8(s.Option), Kind: s.Kind, Tier: s.Tier,
				SkillFrom: int16(s.SkillFrom), SkillTo: int16(s.SkillTo), LearnedFrom: s.LearnedFrom,
				Materials: []RouteMaterial{}, Where: append([]string{}, s.Where...), LearnedBy: []string{}, LearnedNpcIDs: []int32{},
				SourceURL: prof.SourceURL, FetchedAt: c.FetchedAt,
			}
			if s.Recipe != "" {
				name := s.Recipe
				row.RecipeName = &name
				ids := byRecipe[routeKey{strings.ToLower(p), strings.ToLower(name)}]
				// Smelting recipes belong to Mining, whatever page lists them.
				if len(ids) == 0 && strings.HasPrefix(name, "Smelt ") {
					ids = byRecipe[routeKey{"mining", strings.ToLower(name)}]
				}
				row.RecipeSpellID = pickRecipe(ids, s.SkillTo)
				if row.RecipeSpellID == nil {
					rep.UnresolvedRecipes++
					rep.Messages = append(rep.Messages, fmt.Sprintf("%s step %d: recipe %q matches %d recipes", p, s.Step, name, len(ids)))
				}
			}
			if s.Crafts != nil {
				n := int16(*s.Crafts)
				row.Crafts = &n
			}
			for _, m := range s.Materials {
				id := one(byItem[strings.ToLower(m.Item)])
				if id == nil {
					rep.UnresolvedItems++
					rep.Messages = append(rep.Messages, fmt.Sprintf("%s step %d: material %q matches %d items", p, s.Step, m.Item, len(byItem[strings.ToLower(m.Item)])))
				}
				row.Materials = append(row.Materials, RouteMaterial{ItemID: id, Name: m.Item, Count: int16(m.Count)})
			}
			for _, v := range s.LearnedBy {
				label := v.Name
				if v.Place != "" {
					label += " (" + v.Place + ")"
				}
				row.LearnedBy = append(row.LearnedBy, label)
				if id := one(byNpc[strings.ToLower(v.Name)]); id != nil {
					row.LearnedNpcIDs = append(row.LearnedNpcIDs, *id)
				}
			}
			if s.AlternativeOf != nil {
				a := int16(*s.AlternativeOf)
				row.AlternativeOf = &a
			}
			if s.Notes != "" {
				n := s.Notes
				row.Notes = &n
			}
			routes = append(routes, row)
			rep.Rows++
		}
		for _, t := range prof.Tiers {
			row := ProfessionTier{Profession: p, Tier: t.Tier, SkillRequired: t.SkillRequired, MinLevel: t.MinLevel, CostCopper: t.CostCopper,
				How: t.How, SourceURL: prof.SourceURL, FetchedAt: c.FetchedAt}
			if t.Trainer != "" {
				name, place, fac := t.Trainer, t.Place, t.Faction
				row.TrainerName, row.Place, row.Faction = &name, &place, &fac
				id := one(trainerIDs[routeKey{strings.ToLower(p), strings.ToLower(name)}])
				if id == nil {
					id = one(byNpc[strings.ToLower(name)])
				}
				if row.NpcID = id; id == nil {
					rep.UnresolvedTrainers++
				}
			}
			if t.Note != "" {
				n := t.Note
				row.Note = &n
			}
			tiers = append(tiers, row)
		}
	}
	return routes, tiers, rep
}
