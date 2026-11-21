package release

import "strings"

// Notes assembles a release's notes. prevTag names the earlier dataset release and
// changes is the Markdown of what changed since it; with no earlier release (prevTag
// empty) the notes say so instead.
func Notes(build, prevTag, changes string) string {
	var b strings.Builder
	if prevTag == "" {
		b.WriteString("First release: there is no earlier dataset to compare with.\n")
	} else {
		b.WriteString("Changes since " + prevTag + ".\n\n" + strings.TrimRight(changes, "\n") + "\n")
	}
	b.WriteString("\nWoW: Forever dataset for game build " + build + " (Parquet tables plus manifest.json).\n")
	b.WriteString("\nLicence: the dataset is CC BY-SA 4.0 (LICENSE-DATA in the repository).\n")
	b.WriteString("\nCredits:\n")
	b.WriteString("- Game data: Blizzard game client data, retrieved via wago.tools. The underlying game data is Blizzard's; only facts are published.\n")
	b.WriteString("- Loot sources: Data from Everloot (https://everloot.app), CC BY-SA 4.0.\n")
	b.WriteString("\nVerify: `wowc-data verify-manifest <dir>`, or compare the sha256 of each file with manifest.json. See docs/releases.md.\n")
	return b.String()
}
