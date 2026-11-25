// Package history is the small revisions-<build>.json record a release keeps next to its
// revision-named assets: one entry per kept revision, newest first, with each table's rows
// and whether its Everloot source was fetched, carried forward or unavailable.
package history

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/WoW-Companion/data/internal/manifest"
)

const (
	// Keep is how many revisions of a build stay on its release.
	Keep = 10
	// CarriedVersion and UnavailableVersion are the manifest source versions of an Everloot
	// source whose rows were carried forward, or that could not be fetched.
	CarriedVersion     = "carried-forward"
	UnavailableVersion = "unavailable"

	Fetched     = "fetched"
	Carried     = "carried"
	Unavailable = "unavailable"
)

// Entry is one kept revision.
type Entry struct {
	Revision string            `json:"revision"`
	Created  string            `json:"created"`
	Rows     map[string]int64  `json:"rows"`    // by table file name
	Sources  map[string]string `json:"sources"` // Everloot-derived tables: fetched, carried or unavailable
}

// File is revisions-<build>.json.
type File struct {
	Build     string  `json:"build"`
	Revisions []Entry `json:"revisions"` // newest first
}

// AssetName is the name of the history asset of a build.
func AssetName(build string) string { return "revisions-" + build + ".json" }

// ManifestName and ArchiveName are the revision-named copies of a publish.
func ManifestName(build, rev string) string { return "manifest-" + build + "-" + rev + ".json" }
func ArchiveName(build, rev string) string  { return "wowc-dataset-" + build + "-" + rev + ".tar.zst" }

// FromManifest summarises a built dataset as a history entry.
func FromManifest(m manifest.Manifest) Entry {
	e := Entry{Revision: m.Revision, Created: m.Created, Rows: map[string]int64{}, Sources: map[string]string{}}
	for _, t := range m.Tables {
		e.Rows[t.Path] = t.Rows
		for _, s := range t.Sources {
			if !strings.HasPrefix(s.Name, "Everloot") {
				continue
			}
			switch s.Version {
			case UnavailableVersion:
				e.Sources[t.Path] = Unavailable
			case CarriedVersion:
				e.Sources[t.Path] = Carried
			default:
				e.Sources[t.Path] = Fetched
			}
		}
	}
	return e
}

// Parse reads a history file.
func Parse(b []byte) (File, error) {
	var f File
	if err := json.Unmarshal(b, &f); err != nil {
		return f, fmt.Errorf("revisions file: %w", err)
	}
	return f, nil
}

// Marshal encodes the file, indented, with a trailing newline.
func (f File) Marshal() ([]byte, error) {
	b, err := json.MarshalIndent(f, "", "  ")
	return append(b, '\n'), err
}

// Add puts e first (replacing an entry of the same revision) and keeps the newest keep
// entries; pruned lists the revisions that dropped out.
func (f File) Add(e Entry, keep int) (out File, pruned []string) {
	out = File{Build: f.Build}
	out.Revisions = append(out.Revisions, e)
	for _, o := range f.Revisions {
		if o.Revision != e.Revision {
			out.Revisions = append(out.Revisions, o)
		}
	}
	if len(out.Revisions) > keep {
		for _, o := range out.Revisions[keep:] {
			pruned = append(pruned, o.Revision)
		}
		out.Revisions = slices.Clone(out.Revisions[:keep])
	}
	return out, pruned
}

// MaxRows is the most rows any kept revision had, by table file name.
func (f File) MaxRows() map[string]int64 {
	max := map[string]int64{}
	for _, e := range f.Revisions {
		for t, n := range e.Rows {
			max[t] = maxOf(max[t], n)
		}
	}
	return max
}

func maxOf(a, b int64) int64 {
	if b > a {
		return b
	}
	return a
}
