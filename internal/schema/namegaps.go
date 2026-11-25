package schema

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/parquet-go/parquet-go"

	"github.com/WoW-Companion/data/internal/names"
)

// NameGaps lists, per referring table, the item ids it uses that have no name in items or
// names. The check warns; it never fails a build.
type NameGaps map[string][]int32

// Total is the number of distinct unnamed ids over all tables.
func (g NameGaps) Total() int {
	seen := map[int32]bool{}
	for _, ids := range g {
		for _, id := range ids {
			seen[id] = true
		}
	}
	return len(seen)
}

func readRows[T any](dir, file string) ([]T, error) {
	b, err := os.ReadFile(filepath.Join(dir, file))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	rows, err := parquet.Read[T](bytes.NewReader(b), int64(len(b)))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", file, err)
	}
	return rows, nil
}

// ItemNameGaps reads a dataset directory and returns the item ids referenced by
// recipe_reagents, recipes.creates_item_id, vendor_sources and gathering_yields that are
// named by neither items (a non-empty name) nor names (table items).
func ItemNameGaps(dir string) (NameGaps, error) {
	type item struct {
		ID   int32  `parquet:"id"`
		Name string `parquet:"name"`
	}
	items, err := readRows[item](dir, "items.parquet")
	if err != nil {
		return nil, err
	}
	known := map[int32]bool{}
	for _, it := range items {
		if it.Name != "" {
			known[it.ID] = true
		}
	}
	type nameRow struct {
		Table string `parquet:"table"`
		ID    int32  `parquet:"id"`
	}
	nr, err := readRows[nameRow](dir, names.File)
	if err != nil {
		return nil, err
	}
	for _, n := range nr {
		if n.Table == names.Items {
			known[n.ID] = true
		}
	}
	refs := map[string]map[int32]bool{}
	add := func(table string, id int32) {
		if id <= 0 || known[id] {
			return
		}
		if refs[table] == nil {
			refs[table] = map[int32]bool{}
		}
		refs[table][id] = true
	}
	type reagent struct {
		ID int32 `parquet:"reagent_item_id"`
	}
	type recipe struct {
		ID *int32 `parquet:"creates_item_id,optional"`
	}
	type itemRef struct {
		ID int32 `parquet:"item_id"`
	}
	rg, err := readRows[reagent](dir, "recipe_reagents.parquet")
	if err != nil {
		return nil, err
	}
	for _, r := range rg {
		add("recipe_reagents", r.ID)
	}
	rc, err := readRows[recipe](dir, "recipes.parquet")
	if err != nil {
		return nil, err
	}
	for _, r := range rc {
		if r.ID != nil {
			add("recipes", *r.ID)
		}
	}
	for _, t := range []string{"vendor_sources", "gathering_yields"} {
		rows, err := readRows[itemRef](dir, t+".parquet")
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			add(t, r.ID)
		}
	}
	out := NameGaps{}
	for t, set := range refs {
		for id := range set {
			out[t] = append(out[t], id)
		}
		slices.Sort(out[t])
	}
	return out, nil
}

// Markdown renders the gaps for the coverage report.
func (g NameGaps) Markdown() string {
	var b strings.Builder
	b.WriteString("\n## Item name coverage\n\n")
	if len(g) == 0 {
		b.WriteString("Every item referenced by recipe_reagents, recipes.creates_item_id, vendor_sources and gathering_yields has a name.\n")
		return b.String()
	}
	fmt.Fprintf(&b, "WARNING: %d referenced item ids have no name in items or names.\n\n| Table | Unnamed item ids |\n| --- | --- |\n", g.Total())
	tabs := make([]string, 0, len(g))
	for t := range g {
		tabs = append(tabs, t)
	}
	slices.Sort(tabs)
	for _, t := range tabs {
		ids := make([]string, len(g[t]))
		for i, id := range g[t] {
			ids[i] = fmt.Sprint(id)
		}
		fmt.Fprintf(&b, "| %s | %s |\n", t, strings.Join(ids, ", "))
	}
	return b.String()
}
