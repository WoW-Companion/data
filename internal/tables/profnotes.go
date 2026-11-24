package tables

import (
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
)

// ProfessionNote is one row of profession_notes, from curated/professions.yaml.
type ProfessionNote struct {
	Section  string  `parquet:"section,dict"`
	Subject  string  `parquet:"subject,dict"`
	Related  *string `parquet:"related,optional,dict"`
	SkillMin *int16  `parquet:"skill_min,optional"`
	SkillCap *int16  `parquet:"skill_cap,optional"`
	MinLevel *int16  `parquet:"min_level,optional"`
	Text     string  `parquet:"text"`
	Source   string  `parquet:"source,dict"`
}

// ProfessionsSource is every profession_notes row's source.
const ProfessionsSource = "curated (advisor defaults)"

// Sections of curated/professions.yaml: each key lists flow maps with these fields. A field
// name counts as one of the subject (the thing described), related (the other profession),
// numbers, or text.
var professionSections = map[string][]string{
	"professions":   {"name", "kind", "text"},
	"ranks":         {"rank", "skill_min", "skill_cap", "min_level", "text"},
	"pairings":      {"a", "b", "text"},
	"feeders":       {"from", "to", "text"},
	"special_cases": {"profession", "text"},
	"rules":         {"topic", "text"},
}

// professionSectionOrder fixes the row order (alphabetical: it is the table's sort key).
var professionSectionOrder = []string{"feeders", "pairings", "professions", "ranks", "rules", "special_cases"}

// CuratedProfessions is the parsed curated/professions.yaml.
type CuratedProfessions struct {
	Status string
	Rows   []ProfessionNote
	// Names lists every profession name used as a profession (not a rank or topic).
	Names []string
}

// ParseCuratedProfessions reads the YAML subset of the file: a status scalar, then section
// keys each followed by "  - {field: value, ...}" lines. Every row needs every field of its
// section; an unknown section or field, a non-numeric number, or text longer than 400
// characters is an error.
func ParseCuratedProfessions(file, text string) (CuratedProfessions, error) {
	var c CuratedProfessions
	section := ""
	by := map[string][]ProfessionNote{}
	for i, raw := range strings.Split(text, "\n") {
		trim := strings.TrimSpace(raw)
		if trim == "" || strings.HasPrefix(trim, "#") {
			continue
		}
		fail := func(format string, a ...any) error {
			return fmt.Errorf("%s:%d: %s", file, i+1, fmt.Sprintf(format, a...))
		}
		if !strings.HasPrefix(raw, " ") {
			section = ""
			k, v, ok := keyValue(trim)
			switch {
			case ok && k == "status":
				c.Status = unquote(v)
			case ok && v == "" && professionSections[k] != nil:
				section = k
			default:
				return c, fail("unknown top-level line")
			}
			continue
		}
		if section == "" || !strings.HasPrefix(trim, "- {") || !strings.HasSuffix(trim, "}") {
			return c, fail("want a flow map - {field: value, ...} under a section")
		}
		f := map[string]string{}
		for _, part := range splitTop(trim[3:len(trim)-1], ',') {
			k, v, ok := keyValue(part)
			if !ok {
				return c, fail("field %q is not key: value", strings.TrimSpace(part))
			}
			if !slices.Contains(professionSections[section], k) {
				return c, fail("unknown field %q in %s", k, section)
			}
			f[k] = unquote(v)
		}
		for _, k := range professionSections[section] {
			if f[k] == "" {
				return c, fail("%s row needs %s", section, k)
			}
		}
		num := func(k string) (*int16, error) {
			if f[k] == "" {
				return nil, nil
			}
			n, err := strconv.Atoi(f[k])
			if err != nil || n < 0 || n > 600 {
				return nil, fail("%s %q is not a plausible number", k, f[k])
			}
			return ptr16(n), nil
		}
		n := ProfessionNote{Section: section, Text: f["text"], Source: ProfessionsSource}
		if len(n.Text) > 400 {
			return c, fail("text is %d characters; keep notes short", len(n.Text))
		}
		var err error
		switch section {
		case "professions":
			n.Subject = f["name"]
			n.Related = nonEmpty(f["kind"])
			if f["kind"] != "primary" && f["kind"] != "secondary" {
				return c, fail("kind %q must be primary or secondary", f["kind"])
			}
			c.Names = append(c.Names, f["name"])
		case "ranks":
			n.Subject = f["rank"]
			if n.SkillMin, err = num("skill_min"); err != nil {
				return c, err
			}
			if n.SkillCap, err = num("skill_cap"); err != nil {
				return c, err
			}
			if n.MinLevel, err = num("min_level"); err != nil {
				return c, err
			}
			if *n.SkillMin >= *n.SkillCap {
				return c, fail("rank %s: skill_min must be below skill_cap", n.Subject)
			}
		case "pairings":
			n.Subject, n.Related = f["a"], nonEmpty(f["b"])
		case "feeders":
			n.Subject, n.Related = f["from"], nonEmpty(f["to"])
		case "special_cases":
			n.Subject = f["profession"]
		case "rules":
			n.Subject = f["topic"]
		}
		by[section] = append(by[section], n)
	}
	for _, s := range professionSectionOrder {
		c.Rows = append(c.Rows, by[s]...)
	}
	return c, nil
}

func nonEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func ptr16(n int) *int16 { x := int16(n); return &x }

// LoadCuratedProfessions reads, parses and checks the curated profession notes. The file
// must mark itself an advisor default; when skillLines is non-empty every profession named
// in the notes must be one of the client's profession skill line names.
func LoadCuratedProfessions(path string, skillLines []string) (CuratedProfessions, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return CuratedProfessions{}, err
	}
	c, err := ParseCuratedProfessions(path, string(raw))
	if err != nil {
		return c, err
	}
	return c, c.Check(skillLines)
}

// Check applies the cross-checks of LoadCuratedProfessions.
func (c CuratedProfessions) Check(skillLines []string) error {
	if !strings.Contains(c.Status, curatedStatus) {
		return fmt.Errorf("curated professions: status must say %q", curatedStatus)
	}
	if len(skillLines) == 0 {
		return nil
	}
	var bad []string
	chk := func(n string) {
		if n != "" && !slices.Contains(skillLines, n) && !slices.Contains(bad, n) {
			bad = append(bad, n)
		}
	}
	for _, r := range c.Rows {
		switch r.Section {
		case "professions", "special_cases":
			chk(r.Subject)
		case "pairings", "feeders":
			chk(r.Subject)
			if r.Related != nil {
				chk(*r.Related)
			}
		}
	}
	if len(bad) > 0 {
		return fmt.Errorf("curated professions name no client skill line: %s", strings.Join(bad, ", "))
	}
	return nil
}

// ProfessionLineNames is the DisplayName_lang of every profession skill line.
func ProfessionLineNames(skillLines []Row) []string {
	var out []string
	for _, r := range skillLines {
		if isProfessionLine(r) {
			out = append(out, r["DisplayName_lang"])
		}
	}
	return out
}
