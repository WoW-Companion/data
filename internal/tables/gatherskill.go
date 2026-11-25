package tables

// nodeSkill is the curated skill rank a gathering node needs, by the node's exact English
// name. The client tables hold no node (GameObjects has no chest or gathering-node object),
// so no lock can be read for one; these are the published game-mechanics facts, used only
// where the client lock gave nothing. A name not listed stays null.
type nodeSkill struct {
	Profession string
	Skill      int16
}

var curatedNodeSkills = map[string]nodeSkill{
	// Mining
	"Copper Vein":                     {"Mining", 1},
	"Tin Vein":                        {"Mining", 65},
	"Incendicite Mineral Vein":        {"Mining", 65},
	"Lesser Bloodstone Deposit":       {"Mining", 75},
	"Silver Vein":                     {"Mining", 75},
	"Ooze Covered Silver Vein":        {"Mining", 75},
	"Iron Deposit":                    {"Mining", 125},
	"Indurium Mineral Vein":           {"Mining", 150},
	"Gold Vein":                       {"Mining", 155},
	"Ooze Covered Gold Vein":          {"Mining", 155},
	"Mithril Deposit":                 {"Mining", 175},
	"Ooze Covered Mithril Deposit":    {"Mining", 175},
	"Truesilver Deposit":              {"Mining", 230},
	"Ooze Covered Truesilver Deposit": {"Mining", 230},
	"Dark Iron Deposit":               {"Mining", 230},
	"Small Thorium Vein":              {"Mining", 245},
	"Ooze Covered Thorium Vein":       {"Mining", 245},
	"Rich Thorium Vein":               {"Mining", 275},
	"Ooze Covered Rich Thorium Vein":  {"Mining", 275},
	// Herbalism
	"Peacebloom":          {"Herbalism", 1},
	"Silverleaf":          {"Herbalism", 1},
	"Earthroot":           {"Herbalism", 15},
	"Mageroyal":           {"Herbalism", 50},
	"Briarthorn":          {"Herbalism", 70},
	"Stranglekelp":        {"Herbalism", 85},
	"Bruiseweed":          {"Herbalism", 100},
	"Wild Steelbloom":     {"Herbalism", 115},
	"Grave Moss":          {"Herbalism", 120},
	"Kingsblood":          {"Herbalism", 125},
	"Liferoot":            {"Herbalism", 150},
	"Fadeleaf":            {"Herbalism", 150},
	"Goldthorn":           {"Herbalism", 170},
	"Khadgar's Whisker":   {"Herbalism", 185},
	"Wintersbite":         {"Herbalism", 195},
	"Firebloom":           {"Herbalism", 205},
	"Purple Lotus":        {"Herbalism", 210},
	"Arthas' Tears":       {"Herbalism", 220},
	"Sungrass":            {"Herbalism", 230},
	"Blindweed":           {"Herbalism", 235},
	"Ghost Mushroom":      {"Herbalism", 245},
	"Gromsblood":          {"Herbalism", 250},
	"Golden Sansam":       {"Herbalism", 260},
	"Dreamfoil":           {"Herbalism", 270},
	"Mountain Silversage": {"Herbalism", 280},
	"Plaguebloom":         {"Herbalism", 285},
	"Icecap":              {"Herbalism", 290},
	"Black Lotus":         {"Herbalism", 300},
}
