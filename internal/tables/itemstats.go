package tables

import "strconv"

// ItemStat is one stat of an item. Value is the stat amount computed from the level
// budget (Budgets.StatValues); it is null when the budget tables give none.
type ItemStat struct {
	Type  int16  `parquet:"type"`
	Value *int32 `parquet:"value,optional"`
}

// StatTypeRow is one row of the stat_types lookup table.
type StatTypeRow struct {
	ID    int16  `parquet:"id"`
	Label string `parquet:"label"`
}

// StatTypeLabels are the client's item stat enum (ITEM_MOD_*), ids 0 to 56, as short
// labels. The ids above 56 that items carry have no documented label here and are
// left out rather than guessed.
var StatTypeLabels = []StatTypeRow{
	{0, "Mana"}, {1, "Health"}, {3, "Agility"}, {4, "Strength"}, {5, "Intellect"}, {6, "Spirit"}, {7, "Stamina"},
	{12, "Defense Rating"}, {13, "Dodge Rating"}, {14, "Parry Rating"}, {15, "Block Rating"},
	{16, "Melee Hit Rating"}, {17, "Ranged Hit Rating"}, {18, "Spell Hit Rating"},
	{19, "Melee Crit Rating"}, {20, "Ranged Crit Rating"}, {21, "Spell Crit Rating"},
	{22, "Melee Hit Taken Rating"}, {23, "Ranged Hit Taken Rating"}, {24, "Spell Hit Taken Rating"},
	{25, "Melee Crit Taken Rating"}, {26, "Ranged Crit Taken Rating"}, {27, "Spell Crit Taken Rating"},
	{28, "Melee Haste Rating"}, {29, "Ranged Haste Rating"}, {30, "Spell Haste Rating"},
	{31, "Hit Rating"}, {32, "Crit Rating"}, {33, "Hit Taken Rating"}, {34, "Crit Taken Rating"},
	{35, "Resilience Rating"}, {36, "Haste Rating"}, {37, "Expertise Rating"},
	{38, "Attack Power"}, {39, "Ranged Attack Power"}, {40, "Versatility"},
	{41, "Spell Healing Done"}, {42, "Spell Damage Done"}, {43, "Mana Regeneration"},
	{44, "Armor Penetration Rating"}, {45, "Spell Power"}, {46, "Health Regeneration"},
	{47, "Spell Penetration"}, {48, "Block Value"}, {49, "Mastery Rating"}, {50, "Extra Armor"},
	{51, "Fire Resistance"}, {52, "Frost Resistance"}, {53, "Holy Resistance"},
	{54, "Shadow Resistance"}, {55, "Nature Resistance"}, {56, "Arcane Resistance"},
}

// ItemStats lists the item's stat types in slot order, values null; Budgets.StatValues
// fills them.
func ItemStats(r Row) []ItemStat {
	out := []ItemStat{}
	for i := 0; i < statSlots; i++ {
		if s := r.int("StatModifier_bonusStat_"+strconv.Itoa(i), -1); s != -1 {
			out = append(out, ItemStat{Type: int16(s)})
		}
	}
	return out
}

// SpeedMS is the weapon's swing delay in milliseconds from ItemSparse.ItemDelay; null for
// non-weapons and for a zero delay.
func SpeedMS(r Row, classID int) *int32 {
	if classID != 2 {
		return nil
	}
	if d := r.int("ItemDelay", 0); d > 0 {
		return ptr(d)
	}
	return nil
}

// StatTypeTable returns the stat_types lookup rows, sorted by id.
func StatTypeTable() []StatTypeRow { return append([]StatTypeRow(nil), StatTypeLabels...) }
