package bench

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/WoW-Companion/data/internal/names"
	"github.com/WoW-Companion/data/internal/parquetio"
)

type fixture struct {
	ID    int32   `parquet:"id"`
	Name  string  `parquet:"name"`
	Kind  string  `parquet:"kind,dict"`
	Stats []int16 `parquet:"stats,list"`
	Note  *string `parquet:"note,optional"`
}

// TestRunSmoke runs the bench end to end on a small fixture over the local disk and a local
// HTTP server, with no injected latency and no network.
func TestRunSmoke(t *testing.T) {
	dir := t.TempDir()
	rows := make([]fixture, 300)
	for i := range rows {
		rows[i] = fixture{ID: int32(10 + 2*i), Name: "n", Kind: "k", Stats: []int16{1, 2}}
	}
	file := filepath.Join(dir, "fix.parquet")
	if _, _, err := parquetio.WriteFile(file, rows, parquetio.Options{RowGroup: 100, Bloom: []string{"name"}, Sort: []string{"id"}}); err != nil {
		t.Fatal(err)
	}
	ts, err := Run(dir, Config{Local: 5, HTTP: 3})
	if err != nil {
		t.Fatal(err)
	}
	if len(ts) != 1 {
		t.Fatalf("got %d tables", len(ts))
	}
	got := ts[0]
	if got.Rows != 300 || got.RowGroups != 3 || got.Key != "id" || got.DictCols != 1 || len(got.Bloom) != 1 || got.BloomSize == 0 {
		t.Errorf("layout facts wrong: %+v", got)
	}
	for name, s := range map[string]Stat{"point local": got.Point.LocalCold, "point http": got.Point.HTTPCold, "range http warm": got.Range.HTTPWarm} {
		if s.Bytes == 0 || s.Reqs == 0 {
			t.Errorf("%s read nothing: %+v", name, s)
		}
	}
	if md := Markdown("b", Config{}, ts); !strings.Contains(md, "fix.parquet") {
		t.Errorf("markdown lacks the table:\n%s", md)
	}
}

func TestRunNoFiles(t *testing.T) {
	if _, err := Run(t.TempDir(), Config{}); err == nil {
		t.Error("an empty directory must be an error")
	}
}

func TestNamesSmoke(t *testing.T) {
	dir := t.TempDir()
	var e []names.Entry
	for i := 0; i < 250; i++ {
		e = append(e, names.Entry{Table: names.Items, ID: int32(i), Name: "Item " + strings.Repeat("x", i%7) + string(rune('a'+i%26))})
	}
	if _, _, err := parquetio.WriteFile(filepath.Join(dir, names.File), names.Build(e), parquetio.Options{RowGroup: 100, Bloom: []string{"norm"}, Sort: names.SortKeys}); err != nil {
		t.Fatal(err)
	}
	nc, err := Names(dir, Config{Local: 5, HTTP: 3})
	if err != nil {
		t.Fatal(err)
	}
	if nc.Rows == 0 || nc.Exact.HTTPCold.Reqs == 0 || nc.Prefix.LocalCold.Bytes == 0 || !strings.Contains(NamesMarkdown(nc, Config{}), "prefix") {
		t.Errorf("names bench: %+v", nc)
	}
}
