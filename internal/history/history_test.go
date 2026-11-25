package history

import (
	"fmt"
	"testing"

	"github.com/WoW-Companion/data/internal/manifest"
)

func TestAddPrunesAtKeep(t *testing.T) {
	f := File{Build: "1"}
	var pruned []string
	for i := 0; i < Keep+2; i++ {
		f, pruned = f.Add(Entry{Revision: fmt.Sprint("r", i)}, Keep)
	}
	if len(f.Revisions) != Keep || f.Revisions[0].Revision != "r11" || f.Revisions[Keep-1].Revision != "r2" {
		t.Fatalf("kept %+v", f.Revisions)
	}
	if len(pruned) != 1 || pruned[0] != "r1" {
		t.Errorf("pruned %v", pruned)
	}
	// the same revision again replaces itself and moves first, pruning nothing
	f, pruned = f.Add(Entry{Revision: "r5"}, Keep)
	if len(pruned) != 0 || f.Revisions[0].Revision != "r5" || len(f.Revisions) != Keep {
		t.Errorf("republish: %v %+v", pruned, f.Revisions)
	}
}

func TestFromManifestAndMax(t *testing.T) {
	ev := func(v string) []manifest.Source { return []manifest.Source{{Name: "Everloot corpus", Version: v}} }
	m := manifest.Manifest{Revision: "a", Created: "t", Tables: []manifest.Table{
		{Path: "vendor_sources.parquet", Rows: 0, Sources: ev(UnavailableVersion)},
		{Path: "trainers.parquet", Rows: 5, Sources: ev(CarriedVersion)},
		{Path: "items.parquet", Rows: 9, Sources: []manifest.Source{{Name: "wow.tools"}}},
	}}
	e := FromManifest(m)
	if e.Sources["vendor_sources.parquet"] != Unavailable || e.Sources["trainers.parquet"] != Carried || e.Sources["items.parquet"] != "" || e.Rows["items.parquet"] != 9 {
		t.Fatalf("%+v", e)
	}
	f := File{Revisions: []Entry{e, {Revision: "b", Rows: map[string]int64{"vendor_sources.parquet": 2111}}}}
	if got := f.MaxRows()["vendor_sources.parquet"]; got != 2111 {
		t.Errorf("max %d", got)
	}
}
