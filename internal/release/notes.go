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
	b.WriteString("\nVerify: `wowc-data verify-manifest <dir>`, or compare the sha256 of each file with manifest.json. See docs/releases.md.\n")
	return b.String()
}
