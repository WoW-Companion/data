package tables

import "github.com/WoW-Companion/data/internal/names"

// NameIndex builds the rows of names.parquet from the named tables: items, npcs, instances,
// recipes and item sets. Quests carry no name in the client tables, so none are indexed.
func NameIndex(c Cached) []names.Row {
	var e []names.Entry
	for _, r := range c.Items {
		e = append(e, names.Entry{Table: names.Items, ID: r.ID, Name: r.Name})
	}
	for _, r := range c.Npcs {
		e = append(e, names.Entry{Table: names.Npcs, ID: r.ID, Name: r.Name})
	}
	for _, r := range c.Instances {
		e = append(e, names.Entry{Table: names.Instances, ID: r.MapID, Name: r.Name})
	}
	for _, r := range c.Recipes {
		if r.Name != nil {
			e = append(e, names.Entry{Table: names.Recipes, ID: r.SpellID, Name: *r.Name})
		}
	}
	for _, r := range c.ItemSets {
		e = append(e, names.Entry{Table: names.ItemSets, ID: r.SetID, Name: r.Name})
	}
	return names.Build(e)
}
