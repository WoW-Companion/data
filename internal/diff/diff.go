// Package diff summarizes what changed between two dataset versions from their manifests
// and per-column stats, reading Parquet only when a directory has no stats.json.
package diff

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/WoW-Companion/data/internal/manifest"
	"github.com/WoW-Companion/data/internal/schema"
)

// CoverageThreshold is the move, in percentage points, that a column's null or zero
// share must exceed to be reported.
const CoverageThreshold = 5.0

// Dataset is one side of a diff. Stats is nil when the directory holds neither a
// stats.json (the output of `wowc-data coverage -json`) nor Parquet files.
type Dataset struct {
	Manifest manifest.Manifest
	Stats    map[string]schema.TableCoverage // by table name
}

// Load reads dir/manifest.json, then dir/stats.json or, failing that, the Parquet files
// of the registered tables.
func Load(dir string) (Dataset, error) {
	var d Dataset
	b, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return d, err
	}
	if d.Manifest, err = manifest.Decode(b); err != nil {
		return d, fmt.Errorf("%s: %w", dir, err)
	}
	var tcs []schema.TableCoverage
	if sb, err := os.ReadFile(filepath.Join(dir, "stats.json")); err == nil {
		if err := json.Unmarshal(sb, &tcs); err != nil {
			return d, fmt.Errorf("%s/stats.json: %w", dir, err)
		}
	} else if !os.IsNotExist(err) {
		return d, err
	} else if tcs, err = schema.Coverage(dir); err != nil {
		return d, err
	}
	for _, t := range tcs {
		if t.Missing {
			continue
		}
		if d.Stats == nil {
			d.Stats = map[string]schema.TableCoverage{}
		}
		d.Stats[t.Name] = t
	}
	return d, nil
}

func tableName(path string) string { return strings.TrimSuffix(path, ".parquet") }

func rowsOf(m manifest.Manifest) map[string]int64 {
	out := map[string]int64{}
	for _, t := range m.Tables {
		out[tableName(t.Path)] = t.Rows
	}
	return out
}

func union(a, b map[string]int64) []string {
	var names []string
	for n := range a {
		names = append(names, n)
	}
	for n := range b {
		if _, ok := a[n]; !ok {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	return names
}

func share(n, rows int64) float64 {
	if rows == 0 {
		return 0
	}
	return 100 * float64(n) / float64(rows)
}

func rowChange(old, cur int64) string {
	switch {
	case old == cur:
		return "0.0%"
	case old == 0:
		return "n/a"
	}
	return fmt.Sprintf("%+.1f%%", 100*float64(cur-old)/float64(old))
}

// Markdown renders the changes from old to cur.
func Markdown(old, cur Dataset) string {
	var b strings.Builder
	om, cm := old.Manifest, cur.Manifest
	b.WriteString("## Changes since the previous dataset\n\n")
	b.WriteString("| | Old | New |\n| --- | --- | --- |\n")
	fmt.Fprintf(&b, "| dataset_version | %s | %s |\n| game build | %s | %s |\n", om.DatasetVersion, cm.DatasetVersion, om.GameBuild, cm.GameBuild)

	oldRows, newRows := rowsOf(om), rowsOf(cm)
	var added, removed, common []string
	for _, n := range union(oldRows, newRows) {
		_, inOld := oldRows[n]
		_, inNew := newRows[n]
		switch {
		case inNew && !inOld:
			added = append(added, n)
		case inOld && !inNew:
			removed = append(removed, n)
		default:
			common = append(common, n)
		}
	}
	b.WriteString("\n### Tables\n\n")
	list := func(label string, names []string) {
		if len(names) == 0 {
			fmt.Fprintf(&b, "- %s: none\n", label)
			return
		}
		fmt.Fprintf(&b, "- %s: %s\n", label, "`"+strings.Join(names, "`, `")+"`")
	}
	list("Added", added)
	list("Removed", removed)

	b.WriteString("\n### Row counts\n\n| Table | Old | New | Change |\n| --- | ---: | ---: | ---: |\n")
	for _, n := range union(oldRows, newRows) {
		o, c := "-", "-"
		var ov, cv int64
		if v, ok := oldRows[n]; ok {
			o, ov = fmt.Sprint(v), v
		}
		if v, ok := newRows[n]; ok {
			c, cv = fmt.Sprint(v), v
		}
		chg := "n/a"
		if o != "-" && c != "-" {
			chg = rowChange(ov, cv)
		}
		fmt.Fprintf(&b, "| %s | %s | %s | %s |\n", n, o, c, chg)
	}

	b.WriteString("\n### Columns\n\n")
	if old.Stats == nil || cur.Stats == nil {
		b.WriteString("Column and coverage changes are not available: one side has no stats.\n")
		return b.String()
	}
	var cols, cov []string
	for _, n := range added {
		for _, c := range cur.Stats[n].Columns {
			cols = append(cols, fmt.Sprintf("- `%s.%s` added (%s)", n, c.Name, c.Type))
		}
	}
	for _, n := range removed {
		for _, c := range old.Stats[n].Columns {
			cols = append(cols, fmt.Sprintf("- `%s.%s` removed (%s)", n, c.Name, c.Type))
		}
	}
	for _, n := range common {
		ot, ok1 := old.Stats[n]
		ct, ok2 := cur.Stats[n]
		if !ok1 || !ok2 {
			continue
		}
		oc := map[string]schema.ColumnCoverage{}
		for _, c := range ot.Columns {
			oc[c.Name] = c
		}
		seen := map[string]bool{}
		for _, c := range ct.Columns {
			seen[c.Name] = true
			o, ok := oc[c.Name]
			if !ok {
				cols = append(cols, fmt.Sprintf("- `%s.%s` added (%s)", n, c.Name, c.Type))
				continue
			}
			if o.Type != c.Type {
				cols = append(cols, fmt.Sprintf("- `%s.%s` type changed: %s -> %s", n, c.Name, o.Type, c.Type))
			}
			if ot.Rows == 0 || ct.Rows == 0 {
				continue
			}
			for _, k := range []struct {
				label  string
				ov, cv int64
			}{{"null", o.Null, c.Null}, {"zero", o.Zero, c.Zero}} {
				op, cp := share(k.ov, ot.Rows), share(k.cv, ct.Rows)
				if d := cp - op; d > CoverageThreshold || d < -CoverageThreshold {
					cov = append(cov, fmt.Sprintf("- `%s.%s` %s share %.1f%% -> %.1f%% (%+.1f points)", n, c.Name, k.label, op, cp, d))
				}
			}
		}
		for _, c := range ot.Columns {
			if !seen[c.Name] {
				cols = append(cols, fmt.Sprintf("- `%s.%s` removed (%s)", n, c.Name, c.Type))
			}
		}
	}
	section := func(items []string, none string) {
		if len(items) == 0 {
			b.WriteString(none + "\n")
			return
		}
		b.WriteString(strings.Join(items, "\n") + "\n")
	}
	section(cols, "No columns added, removed or type-changed.")
	fmt.Fprintf(&b, "\n### Coverage moves over %.0f points\n\n", CoverageThreshold)
	section(cov, "None.")
	return b.String()
}
