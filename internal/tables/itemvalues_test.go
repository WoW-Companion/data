package tables

import (
	"reflect"
	"testing"
)

// fixtureBudgets holds the client rows for item levels 10 and 2, copied from build 1.60.1.71091.
func fixtureBudgets() *Budgets {
	return NewBudgets(map[string][]Row{
		"RandPropPoints":    {{"ID": "10", "Good_0": "1", "Good_1": "1", "Good_2": "1", "Good_3": "1", "Superior_1": "4", "Epic_1": "7"}},
		"ItemArmorTotal":    {{"ItemLevel": "10", "Cloth": "120.3", "Leather": "343.22", "Mail": "673.61", "Plate": "590.81"}},
		"ItemArmorQuality":  {{"ID": "10", "Qualitymod_2": "1", "Qualitymod_3": "1.1"}},
		"ItemArmorShield":   {{"ItemLevel": "10", "Quality_2": "40"}},
		"ArmorLocation":     {{"ID": "5", "Chainmodifier": "0.16", "Clothmodifier": "0.16"}, {"ID": "8", "Chainmodifier": "0.11"}, {"ID": "2"}},
		"ItemDamageOneHand": {{"ItemLevel": "2", "Quality_1": "1.01635062695"}},
	})
}

func TestStatValues(t *testing.T) {
	b := fixtureBudgets()
	boots := Row{"StatModifier_bonusStat_0": "7", "StatPercentEditor_0": "10000"}
	two := Row{"StatModifier_bonusStat_0": "4", "StatPercentEditor_0": "5000", "StatModifier_bonusStat_1": "-1", "StatModifier_bonusStat_2": "7", "StatPercentEditor_2": "0"}
	cases := []struct {
		name               string
		row                Row
		ilvl, quality, inv int
		want               []int32 // -1 for null
	}{
		{"copper chain boots: feet reads column 1", boots, 10, 2, 8, []int32{1}},
		{"rare, same slot", boots, 10, 3, 8, []int32{4}},
		{"half share rounds, empty slot skipped, zero share null", two, 10, 3, 8, []int32{2, -1}},
		{"common item has no budget", boots, 10, 1, 8, []int32{-1}},
		{"level with no row", boots, 11, 2, 8, []int32{-1}},
		{"slot with no budget", boots, 10, 2, 19, []int32{-1}},
	}
	for _, c := range cases {
		stats := ItemStats(c.row)
		b.StatValues(c.row, stats, c.ilvl, c.quality, c.inv, 4, 3)
		var got []int32
		for _, s := range stats {
			if s.Value == nil {
				got = append(got, -1)
			} else {
				got = append(got, *s.Value)
			}
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
	var nilB *Budgets
	stats := ItemStats(boots)
	nilB.StatValues(boots, stats, 10, 2, 8, 4, 3)
	if stats[0].Value != nil {
		t.Error("nil budgets must fill nothing")
	}
}

func TestArmorOf(t *testing.T) {
	b := fixtureBudgets()
	cases := []struct {
		name                         string
		ilvl, quality, inv, cls, sub int
		want                         int32 // 0 for null
	}{
		{"copper chain boots", 10, 2, 8, 4, 3, 74},
		{"rough copper vest", 10, 2, 5, 4, 3, 108},
		{"robe reads the chest row", 10, 2, 20, 4, 1, 19},
		{"quality modifier", 10, 3, 8, 4, 3, 82},
		{"shield", 10, 2, 14, 4, 6, 40},
		{"jewellery has no armor", 10, 2, 2, 4, 0, 0},
		{"weapon", 10, 2, 8, 2, 3, 0},
		{"level with no row", 11, 2, 8, 4, 3, 0},
	}
	for _, c := range cases {
		got := b.ArmorOf(c.ilvl, c.quality, c.inv, c.cls, c.sub)
		if (got == nil) != (c.want == 0) || (got != nil && *got != c.want) {
			t.Errorf("%s: got %v want %d", c.name, got, c.want)
		}
	}
}

func TestDamageOf(t *testing.T) {
	b := fixtureBudgets()
	sword := Row{"ItemDelay": "1900", "DmgVariance": "0.6"}
	lo, hi := b.DamageOf(sword, 2, 1, 13, 2, 7)
	if lo == nil || hi == nil || *lo != 1 || *hi != 3 {
		t.Errorf("worn shortsword: got %v %v want 1 3", lo, hi)
	}
	for name, c := range map[string]struct {
		row        Row
		inv, class int
	}{
		"no delay":    {Row{"DmgVariance": "0.6"}, 13, 2},
		"not weapon":  {sword, 13, 4},
		"no table":    {sword, 19, 2},
		"caster flag": {Row{"ItemDelay": "1900", "Flags_1": "512"}, 13, 2},
	} {
		if lo, _ := b.DamageOf(c.row, 2, 1, c.inv, c.class, 7); lo != nil {
			t.Errorf("%s: want null, got %d", name, *lo)
		}
	}
}
