package release

import (
	"strings"
	"testing"

	"github.com/WoW-Companion/data/internal/manifest"
)

func mf(rows map[string]int64) manifest.Manifest {
	m := manifest.Manifest{Revision: "fe158bb68a7d"}
	for p, n := range rows {
		m.Tables = append(m.Tables, manifest.Table{Path: p, Rows: n})
	}
	return m
}

func TestCheckShrink(t *testing.T) {
	prev := mf(map[string]int64{"vendor_sources.parquet": 2111, "items.parquet": 100, "trainers.parquet": 0})
	cases := []struct {
		name  string
		cur   map[string]int64
		allow []Allow
		want  string // substring of the error, "" for none
	}{
		{"to zero", map[string]int64{"vendor_sources.parquet": 0, "items.parquet": 100}, nil, "table vendor_sources would shrink from 2111 rows to 0"},
		{"over half", map[string]int64{"vendor_sources.parquet": 1000, "items.parquet": 100}, nil, "from 2111 rows to 1000"},
		{"exactly half", map[string]int64{"vendor_sources.parquet": 1056, "items.parquet": 50}, nil, ""},
		{"grew, empty stays empty", map[string]int64{"vendor_sources.parquet": 3000, "items.parquet": 100, "trainers.parquet": 0}, nil, ""},
		{"dropped table", map[string]int64{"vendor_sources.parquet": 2111}, nil, "table items would shrink from 100 rows to 0"},
		{"allowed", map[string]int64{"vendor_sources.parquet": 0, "items.parquet": 100}, []Allow{{"vendor_sources", "feed retired"}}, ""},
	}
	for _, c := range cases {
		err := CheckShrink(prev, mf(c.cur), c.allow)
		if (err == nil) != (c.want == "") || (err != nil && !strings.Contains(err.Error(), c.want)) {
			t.Errorf("%s: err = %v, want %q", c.name, err, c.want)
		}
	}
}

func TestParseAllowShrink(t *testing.T) {
	got, err := ParseAllowShrink("# why\nallow_shrink:\n  - table: vendor_sources\n    reason: \"feed retired\"\n  - table: items.parquet\n    reason: rebuilt\n")
	if err != nil || len(got) != 2 || got[0] != (Allow{"vendor_sources", "feed retired"}) || got[1].Table != "items.parquet" {
		t.Fatalf("%+v %v", got, err)
	}
	if _, err := ParseAllowShrink("allow_shrink:\n  - table: items\n"); err == nil {
		t.Error("an entry without a reason must be refused")
	}
	if got, err := ParseAllowShrink("allow_shrink: []\n"); err != nil || len(got) != 0 {
		t.Errorf("empty list: %+v %v", got, err)
	}
}
