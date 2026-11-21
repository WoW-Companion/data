package names

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/WoW-Companion/data/internal/parquetio"
)

func TestNormalize(t *testing.T) {
	for in, want := range map[string]string{
		"Thunderfury, Blessed Blade of the Windseeker": "thunderfury blessed blade of the windseeker",
		"Thunderfury’s Edge":                           "thunderfurys edge",
		"Séance  of the\tÉlite":                        "seance of the elite",
		"Hogger (Elite) - 2":                           "hogger elite 2",
		"  ...  ":                                      "",
		"":                                             "",
	} {
		if got := Normalize(in); got != want {
			t.Errorf("Normalize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestBuildSortsAndDedupes(t *testing.T) {
	got := Build([]Entry{{Items, 2, "Zed"}, {Npcs, 1, "Zed"}, {Items, 2, "Zed"}, {Items, 1, "ábc"}, {Items, 9, "!!"}})
	want := []Row{{"abc", "ábc", Items, 1}, {"zed", "Zed", Items, 2}, {"zed", "Zed", Npcs, 1}}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// A file of several row groups: an exact name and a prefix are found, and a lookup that the
// min/max range or Bloom filter rules out reads no row.
func TestLookup(t *testing.T) {
	var e []Entry
	for i := 0; i < 300; i++ {
		e = append(e, Entry{Items, int32(i), fmt.Sprintf("Item %03d", i)}, Entry{Npcs, int32(i), fmt.Sprintf("Item %03d", i)})
	}
	b, err := parquetio.Encode(Build(e), parquetio.Options{RowGroup: 100, Bloom: []string{"norm"}, Sort: SortKeys})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		q      string
		prefix bool
		limit  int
		want   int
	}{
		{"item 150", false, 10, 2},
		{"item 15", true, 100, 20},
		{"item 15", true, 5, 5},
		{"item 15", false, 10, 0},
		{"item 1500", false, 10, 0},
		{"zzz", true, 10, 0},
		{"a", true, 10, 0},
		{"item", true, 1000, 600},
	} {
		got, err := Lookup(bytes.NewReader(b), int64(len(b)), c.q, c.prefix, c.limit)
		if err != nil || len(got) != c.want {
			t.Errorf("Lookup(%q, prefix=%v, %d) = %d rows, %v; want %d", c.q, c.prefix, c.limit, len(got), err, c.want)
		}
	}
	all, err := ReadAll(b)
	if err != nil || len(all) != 600 {
		t.Errorf("ReadAll: %d rows, %v", len(all), err)
	}
}
