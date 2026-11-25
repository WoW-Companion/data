package tables

import (
	"slices"
	"strconv"
	"strings"
)

// Random suffix kinds: the client table a suffix comes from.
const (
	KindProperties = "properties" // ItemRandomProperties: fixed enchants, amounts in SpellItemEnchantment
	KindSuffix     = "suffix"     // ItemRandomSuffix: enchants scaled by the item's level budget
)

// enchantStatEffect is SpellItemEnchantment.Effect_n for an item stat (ITEM_MOD) bonus;
// EffectArg_n then holds the stat type, the same ids as items.stat_types.
const enchantStatEffect = 5

// RandomSuffix is one random suffix name ("of the Eagle") with the stats it gives, one row
// per client table, name and set of stats. Amounts is aligned with Stats and is empty when
// the client holds no fixed amount (suffix kind: the amount scales with the item).
type RandomSuffix struct {
	SuffixID int32   `parquet:"suffix_id"`
	Name     string  `parquet:"name"`
	Stats    []int16 `parquet:"stats"`
	Amounts  []int32 `parquet:"amounts"`
	Kind     string  `parquet:"kind"`
	Source   string  `parquet:"source"`
	IDs      []int32 `parquet:"ids"`
}

// RandomSuffixInput are the client tables a build reads: the two suffix tables and the
// enchantments they point at.
type RandomSuffixInput struct {
	Properties, Suffixes, Enchants []Row
}

type enchantStat struct {
	stat   int16
	amount int32
}

func enchantStats(e Row) []enchantStat {
	var out []enchantStat
	for j := 0; j < 3; j++ {
		s := strconv.Itoa(j)
		if e.int("Effect_"+s, 0) != enchantStatEffect {
			continue
		}
		out = append(out, enchantStat{int16(e.int("EffectArg_"+s, 0)), int32(e.int("EffectPointsMin_"+s, 0))})
	}
	return out
}

// BuildRandomSuffixes lists the suffixes of ItemRandomProperties and ItemRandomSuffix. A
// suffix's stats are the stat enchants (Effect 5) of its Enchantment_0..4, deduplicated and
// sorted by stat id. The result is sorted by kind, then lowest id. Rows of the same table with the same name and the same stats are one
// row: SuffixID is the lowest id and IDs lists them all. Names with no stat enchant (all
// its enchants are resistances or spells) are left out. Amounts are filled for the
// properties kind when every stat has a nonzero amount in every grouped suffix and they agree;
// where the grouped suffixes differ in amount, the largest is kept.
func BuildRandomSuffixes(in RandomSuffixInput) []RandomSuffix {
	enchants := make(map[int][]enchantStat, len(in.Enchants))
	for _, e := range in.Enchants {
		enchants[e.int("ID", 0)] = enchantStats(e)
	}
	type group struct {
		row     RandomSuffix
		amounts map[int16]int32
		fixed   bool
	}
	groups := map[string]*group{}
	var order []string
	add := func(rows []Row, kind, source string) {
		for _, r := range rows {
			name := strings.TrimSpace(r["Name_lang"])
			if name == "" {
				continue
			}
			amt := map[int16]int32{}
			for i := 0; i < 5; i++ {
				for _, es := range enchants[r.int("Enchantment_"+strconv.Itoa(i), 0)] {
					amt[es.stat] += es.amount
				}
			}
			if len(amt) == 0 {
				continue
			}
			stats := make([]int16, 0, len(amt))
			for s := range amt {
				stats = append(stats, s)
			}
			slices.Sort(stats)
			var key strings.Builder
			key.WriteString(kind + "|" + name)
			for _, s := range stats {
				key.WriteString("|" + strconv.Itoa(int(s)))
			}
			id := int32(r.int("ID", 0))
			g := groups[key.String()]
			if g == nil {
				g = &group{row: RandomSuffix{SuffixID: id, Name: name, Stats: stats, Kind: kind, Source: source}, amounts: map[int16]int32{}, fixed: kind == KindProperties}
				groups[key.String()] = g
				order = append(order, key.String())
			}
			g.row.IDs = append(g.row.IDs, id)
			g.row.SuffixID = min(g.row.SuffixID, id)
			for _, s := range stats {
				if amt[s] <= 0 {
					g.fixed = false
				}
				g.amounts[s] = max(g.amounts[s], amt[s])
			}
		}
	}
	add(in.Properties, KindProperties, "ItemRandomProperties")
	add(in.Suffixes, KindSuffix, "ItemRandomSuffix")
	out := make([]RandomSuffix, 0, len(order))
	for _, k := range order {
		g := groups[k]
		slices.Sort(g.row.IDs)
		g.row.Amounts = []int32{}
		if g.fixed {
			for _, s := range g.row.Stats {
				g.row.Amounts = append(g.row.Amounts, g.amounts[s])
			}
		}
		out = append(out, g.row)
	}
	slices.SortStableFunc(out, func(a, b RandomSuffix) int {
		if c := strings.Compare(a.Kind, b.Kind); c != 0 {
			return c
		}
		if a.SuffixID != b.SuffixID {
			return int(a.SuffixID) - int(b.SuffixID)
		}
		return strings.Compare(a.Name, b.Name)
	})
	return out
}
