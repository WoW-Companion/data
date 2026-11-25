package release

import (
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/WoW-Companion/data/internal/manifest"
)

// AllowShrinkFile is the committed list of tables that may shrink on purpose.
const AllowShrinkFile = "curated/allow-shrink.yaml"

// Allow is one allow_shrink entry: a table and why it may shrink.
type Allow struct{ Table, Reason string }

// ParseAllowShrink reads the YAML subset of curated/allow-shrink.yaml:
//
//	allow_shrink:
//	  - table: vendor_sources
//	    reason: Everloot retired the vendor feed
//
// An entry without a table or without a reason is an error.
func ParseAllowShrink(text string) ([]Allow, error) {
	var out []Allow
	var cur *Allow
	flush := func() error {
		if cur == nil {
			return nil
		}
		if cur.Table == "" || cur.Reason == "" {
			return fmt.Errorf("allow_shrink entry %q needs both a table and a reason", cur.Table)
		}
		out = append(out, *cur)
		cur = nil
		return nil
	}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || line == "allow_shrink:" || line == "allow_shrink: []" {
			continue
		}
		if rest, ok := strings.CutPrefix(line, "- "); ok {
			if err := flush(); err != nil {
				return nil, err
			}
			cur = &Allow{}
			line = strings.TrimSpace(rest)
		}
		k, v, ok := strings.Cut(line, ":")
		if !ok || cur == nil {
			return nil, fmt.Errorf("allow-shrink: cannot read %q", line)
		}
		v = strings.Trim(strings.TrimSpace(v), `"'`)
		switch strings.TrimSpace(k) {
		case "table":
			cur.Table = v
		case "reason":
			cur.Reason = v
		default:
			return nil, fmt.Errorf("allow-shrink: unknown key %q", k)
		}
	}
	return out, flush()
}

// LoadAllowShrink reads path; a missing file allows nothing.
func LoadAllowShrink(path string) ([]Allow, error) {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return ParseAllowShrink(string(b))
}

// CheckShrink refuses a dataset that drops a table that had rows by more than 50%, or to
// 0 (or drops the table), against the baseline (the previous release, or the maximum over the kept revisions), unless allow lists the table. The
// error names every such table with both counts.
func CheckShrink(prev, cur manifest.Manifest, allow []Allow) error {
	now := map[string]int64{}
	for _, t := range cur.Tables {
		now[t.Path] = t.Rows
	}
	var bad []string
	for _, p := range prev.Tables {
		if p.Rows == 0 {
			continue
		}
		n := now[p.Path]
		if n*2 >= p.Rows {
			continue
		}
		name := strings.TrimSuffix(p.Path, ".parquet")
		if slices.ContainsFunc(allow, func(a Allow) bool { return strings.TrimSuffix(a.Table, ".parquet") == name }) {
			continue
		}
		bad = append(bad, fmt.Sprintf("table %s would shrink from %d rows to %d", name, p.Rows, n))
	}
	if len(bad) == 0 {
		return nil
	}
	return fmt.Errorf("refusing to publish: %s (more than 50%% fewer rows than %s); if intended, add the table with a reason to %s",
		strings.Join(bad, "; "), prev.Revision, AllowShrinkFile)
}
