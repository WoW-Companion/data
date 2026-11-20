package parquetio

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/WoW-Companion/data/internal/manifest"
	"github.com/parquet-go/parquet-go"
)

type row struct {
	ID          int32  `parquet:"id"`
	QuestLineID *int32 `parquet:"quest_line_id,optional"`
}

// TestRoundTrip is the smoke test: rows, sort order, row groups and provenance survive a write.
func TestRoundTrip(t *testing.T) {
	one := int32(1)
	rows := []row{{ID: 1, QuestLineID: &one}, {ID: 2}, {ID: 3}, {ID: 4}, {ID: 5}}
	src := []manifest.Source{{Name: "n", Version: "v", Retrieved: "2026-11-20T00:00:00Z", License: "l"}}
	b, err := Encode(rows, Options{RowGroup: 2, Bloom: []string{"id"}, Sort: []string{"id"}, Sources: src})
	if err != nil {
		t.Fatal(err)
	}
	got, err := parquet.Read[row](bytes.NewReader(b), int64(len(b)))
	if err != nil || !reflect.DeepEqual(got, rows) {
		t.Fatalf("rows differ: %v %v", got, err)
	}
	f, _ := parquet.OpenFile(bytes.NewReader(b), int64(len(b)))
	if n := len(f.RowGroups()); n != 3 {
		t.Errorf("row groups = %d, want 3", n)
	}
	if f.RowGroups()[0].ColumnChunks()[0].BloomFilter() == nil {
		t.Error("no Bloom filter on id")
	}
	gs, err := ReadSources(b)
	if err != nil || !reflect.DeepEqual(gs, src) {
		t.Errorf("sources = %v, %v", gs, err)
	}
}
