package release

import "strings"

// Notes assembles a release's notes. prevTag names the earlier dataset release and
// changes is the Markdown of what changed since it; with no earlier release (prevTag
// empty) the notes say so instead.
func Notes(build, prevTag, changes string) string {
	return NotesAt(build, prevTag, changes, "", "")
}

// NotesAt is Notes with a dated "Revision" line when revision is not empty: the hash of
// the curated files, table code and schemas the dataset was built from, and the date.
func NotesAt(build, prevTag, changes, revision, date string) string {
	var b strings.Builder
	if prevTag == "" {
		b.WriteString("First release: there is no earlier dataset to compare with.\n")
	} else {
		b.WriteString("Changes since " + prevTag + ".\n\n" + strings.TrimRight(changes, "\n") + "\n")
	}
	b.WriteString("\nWoW: Forever dataset for game build " + build + " (Parquet tables plus manifest.json).\n")
	if revision != "" {
		b.WriteString("\nRevision: " + revision + " (" + date + ").\n")
	}
	b.WriteString("\nLicence: the dataset is CC BY-SA 4.0 (LICENSE-DATA in the repository).\n")
	b.WriteString("\nCredits:\n")
	b.WriteString("- Game data: Blizzard game client data, retrieved via wago.tools. The underlying game data is Blizzard's; only facts are published.\n")
	b.WriteString("- Loot sources: Data from Everloot (https://everloot.app), CC BY-SA 4.0.\n")
	b.WriteString("- Observed quests (quests_observed) and vendor offers (vendor_sources): Data from Everloot (https://everloot.app), CC BY-SA 4.0.\n")
	b.WriteString("- Profession leveling routes (profession_routes, profession_tiers): facts (skill ranges, recipes, craft counts, materials, trainers) read from the WoW Forever leveling guides on wow-professions.com (https://www.wow-professions.com/forever/); no guide text is published.\n")
	b.WriteString("- Talent builds and key talents (talent_builds, talent_priorities): follow the Icy Veins WoW Forever leveling guides (https://www.icy-veins.com/wow-forever/); rows not read from a guide are marked curated (unverified).\n")
	b.WriteString("- UI maps (ui_maps): facts (ids, names, types, parents) from the client tables UiMap and UiMapAssignment, via wago.tools; no text is published beyond map names.\n")
	b.WriteString("\nVerify: `wowc-data verify-manifest <dir>`, or compare the sha256 of each file with manifest.json. See docs/releases.md.\n")
	return b.String()
}
