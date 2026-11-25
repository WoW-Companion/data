package tables

import (
	"math"
	"strconv"
)

// Budgets holds the parsed budget tables. A nil or empty Budgets fills nothing.
type Budgets struct {
	randProp  map[int]Row // item level -> RandPropPoints row
	armor     map[int]Row // item level -> ItemArmorTotal row
	armorQual map[int]Row // item level -> ItemArmorQuality row
	shield    map[int]Row // item level -> ItemArmorShield row
	location  map[int]Row // inventory type -> ArmorLocation row
	damage    map[string]map[int]Row
}

// NewBudgets indexes the client tables by item level (or inventory type for ArmorLocation).
// rows is keyed by table name, as LoadCache reads them.
func NewBudgets(rows map[string][]Row) *Budgets {
	b := &Budgets{
		randProp:  byKey(rows["RandPropPoints"], "ID"),
		armor:     byKey(rows["ItemArmorTotal"], "ItemLevel"),
		armorQual: byKey(rows["ItemArmorQuality"], "ID"),
		shield:    byKey(rows["ItemArmorShield"], "ItemLevel"),
		location:  byKey(rows["ArmorLocation"], "ID"),
		damage:    map[string]map[int]Row{},
	}
	for _, t := range []string{"ItemDamageOneHand", "ItemDamageOneHandCaster", "ItemDamageTwoHand", "ItemDamageTwoHandCaster", "ItemDamageRanged", "ItemDamageThrown", "ItemDamageWand"} {
		b.damage[t] = byKey(rows[t], "ItemLevel")
	}
	return b
}

func byKey(rows []Row, key string) map[int]Row {
	m := make(map[int]Row, len(rows))
	for _, r := range rows {
		m[r.int(key, 0)] = r
	}
	return m
}

func (r Row) float(key string) float64 {
	f, err := strconv.ParseFloat(r[key], 64)
	if err != nil {
		return 0
	}
	return f
}

func round(f float64) int32 { return int32(math.Floor(f + 0.5)) }

// Inventory types.
const (
	invHead, invNeck, invShoulders, invBody, invChest, invWaist, invLegs, invFeet = 1, 2, 3, 4, 5, 6, 7, 8
	invWrists, invHands, invFinger, invTrinket, invWeapon, invShield, invRanged   = 9, 10, 11, 12, 13, 14, 15
	invCloak, inv2H, invHoldable, invRobe, invMainHand, invOffHand, invThrown     = 16, 17, 23, 20, 21, 22, 25
	invRangedRight                                                                = 26
)

const subclassWand = 19

// budgetSlot is the RandPropPoints column (0 to 4) an item's stat budget is read from, by
// inventory type; -1 when the type carries no budget.
func budgetSlot(inv, classID, subclass int) int {
	switch inv {
	case invHead, invBody, invChest, invLegs, invRanged, inv2H, invRobe, invThrown:
		return 0
	case invRangedRight:
		if classID == 2 && subclass == subclassWand {
			return 3
		}
		return 0
	case invShoulders, invWaist, invFeet, invHands, invTrinket:
		return 1
	case invNeck, invWrists, invFinger, invShield, invCloak, invHoldable:
		return 2
	case invWeapon, invMainHand, invOffHand:
		return 3
	}
	return -1
}

// budgetColumn is the RandPropPoints column family for a quality: Good for uncommon,
// Superior for rare, Epic for epic and above (and heirloom). Poor and common items have
// no budget.
func budgetColumn(quality int) string {
	switch {
	case quality == 2:
		return "Good_"
	case quality == 3:
		return "Superior_"
	case quality >= 4:
		return "Epic_"
	}
	return ""
}

// StatBudget is the stat budget of an item: RandPropPoints for its level, quality and
// slot. ok is false when the tables give none.
func (b *Budgets) StatBudget(ilvl, quality, inv, classID, subclass int) (float64, bool) {
	col, slot := budgetColumn(quality), budgetSlot(inv, classID, subclass)
	if b == nil || col == "" || slot < 0 {
		return 0, false
	}
	row, ok := b.randProp[ilvl]
	if !ok {
		return 0, false
	}
	v := row.float(col + strconv.Itoa(slot))
	return v, v > 0
}

// StatValues fills each stat's amount: StatPercentEditor / 10000 of the item's stat
// budget, rounded to the nearest whole number (the client's own rule for this table
// generation). A stat with a zero share, or an item with no budget, stays null. Gem
// socket cost, which the client subtracts on socketed items, is not modelled: this build
// has no socket cost table.
func (b *Budgets) StatValues(r Row, stats []ItemStat, ilvl, quality, inv, classID, subclass int) {
	budget, ok := b.StatBudget(ilvl, quality, inv, classID, subclass)
	if !ok {
		return
	}
	i := 0
	for slot := 0; slot < statSlots; slot++ {
		if r.int("StatModifier_bonusStat_"+strconv.Itoa(slot), -1) == -1 {
			continue
		}
		if pct := r.int("StatPercentEditor_"+strconv.Itoa(slot), 0); pct != 0 {
			v := round(float64(pct) * budget / 10000)
			stats[i].Value = &v
		}
		i++
	}
}

// ArmorOf is the item's armor: the level's total armor for its material, times the
// quality modifier, times the slot's share (ArmorLocation); shields read ItemArmorShield.
// Null for items that carry none (jewellery, relics, a zero result).
func (b *Budgets) ArmorOf(ilvl, quality, inv, classID, subclass int) *int32 {
	if b == nil || classID != 4 || quality < 0 || quality > 6 {
		return nil
	}
	var v int32
	if inv == invShield {
		if subclass != 6 {
			return nil
		}
		v = round(b.shield[ilvl].float("Quality_" + strconv.Itoa(quality)))
	} else {
		mats := []string{"Cloth", "Leather", "Mail", "Plate"}
		if subclass < 1 || subclass > 4 {
			return nil
		}
		total := b.armor[ilvl].float(mats[subclass-1])
		mod := b.armorQual[ilvl].float("Qualitymod_" + strconv.Itoa(quality))
		if inv == invRobe {
			inv = invChest // ArmorLocation has no robe row: a robe is a chest piece
		}
		loc := b.location[inv].float([]string{"Clothmodifier", "Leathermodifier", "Chainmodifier", "Platemodifier"}[subclass-1])
		v = round(total * mod * loc)
	}
	if v <= 0 {
		return nil
	}
	return &v
}

// damageTable names the client damage table for a weapon.
func damageTable(inv, classID, subclass int, caster bool) string {
	switch inv {
	case inv2H:
		if caster {
			return "ItemDamageTwoHandCaster"
		}
		return "ItemDamageTwoHand"
	case invRanged, invRangedRight:
		if subclass == subclassWand {
			return "ItemDamageWand"
		}
		return "ItemDamageRanged"
	case invThrown:
		return "ItemDamageThrown"
	case invWeapon, invMainHand, invOffHand:
		if caster {
			return "ItemDamageOneHandCaster"
		}
		return "ItemDamageOneHand"
	}
	return ""
}

// casterWeaponFlag is the item flag (second flags word) that marks a weapon as a caster
// weapon, which reads the caster damage table.
const casterWeaponFlag = 0x200

// DamageOf is the weapon's damage range: the level's damage per second for its type and
// quality, times the swing delay, spread by DmgVariance (min = avg x (1 - v/2), max = avg
// x (1 + v/2)), each rounded. Null for non-weapons and items with no delay or no table.
func (b *Budgets) DamageOf(r Row, ilvl, quality, inv, classID, subclass int) (lo, hi *int32) {
	delay := r.int("ItemDelay", 0)
	if b == nil || classID != 2 || delay <= 0 || quality < 0 || quality > 6 {
		return nil, nil
	}
	t := damageTable(inv, classID, subclass, r.int("Flags_1", 0)&casterWeaponFlag != 0)
	if t == "" {
		return nil, nil
	}
	dps := b.damage[t][ilvl].float("Quality_" + strconv.Itoa(quality))
	if dps <= 0 {
		return nil, nil
	}
	avg, v := dps*float64(delay)/1000, r.float("DmgVariance")
	l, h := round(avg*(1-v/2)), round(avg*(1+v/2))
	return &l, &h
}
