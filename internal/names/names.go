// Package names builds and reads names.parquet, the dataset's prebuilt name index: one row
// per named item, creature, instance, recipe and item set, sorted by the normalised name so a
// reader finds an exact name or a prefix without scanning any table.
package names

import (
	"bytes"
	"cmp"
	"io"
	"slices"
	"strings"
	"unicode"

	"github.com/parquet-go/parquet-go"
	"golang.org/x/text/unicode/norm"
)

// File is the index's file name in a dataset directory.
const File = "names.parquet"

// SortKeys is the order of the rows; norm leads, so prefix and exact reads prune by it.
var SortKeys = []string{"norm", "table", "id"}

// Table values of Row.Table.
const (
	Items     = "items"
	Npcs      = "npcs"
	Instances = "instances"
	Recipes   = "recipes"
	ItemSets  = "item_sets"
)

// Row is one name of the index. Table and ID point at the row in that table: items.id,
// npcs.id, instances.map_id, recipes.spell_id, item_sets.set_id.
type Row struct {
	Norm  string `parquet:"norm"`
	Name  string `parquet:"name"`
	Table string `parquet:"table,dict"`
	ID    int32  `parquet:"id"`
}

// Normalize lowercases a name, folds accents (é to e) and drops punctuation; letters and
// digits stay and every run of other characters becomes one space, trimmed at both ends.
func Normalize(s string) string {
	var b strings.Builder
	space := false
	for _, r := range norm.NFD.String(strings.ToLower(s)) {
		switch {
		case unicode.Is(unicode.Mn, r):
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			if space && b.Len() > 0 {
				b.WriteByte(' ')
			}
			space = false
			b.WriteRune(r)
		case r == '\'' || r == '’':
			// "Thunderfury's" and "Thunderfurys" are one word
		default:
			space = true
		}
	}
	return b.String()
}

// Entry is one name to index: the table it names a row of and that row's id.
type Entry struct {
	Table string
	ID    int32
	Name  string
}

// Build returns the index rows of the entries, sorted by (norm, table, id) with exact repeats
// removed. Entries whose name normalises to nothing are skipped.
func Build(entries []Entry) []Row {
	var out []Row
	for _, e := range entries {
		if n := Normalize(e.Name); n != "" {
			out = append(out, Row{Norm: n, Name: e.Name, Table: e.Table, ID: e.ID})
		}
	}
	slices.SortFunc(out, func(a, b Row) int {
		return cmp.Or(cmp.Compare(a.Norm, b.Norm), cmp.Compare(a.Table, b.Table), cmp.Compare(a.ID, b.ID), cmp.Compare(a.Name, b.Name))
	})
	return slices.CompactFunc(out, func(a, b Row) bool { return a.Norm == b.Norm && a.Table == b.Table && a.ID == b.ID })
}

// Lookup returns up to limit rows whose norm equals q (exact) or starts with q (prefix),
// reading only the row groups whose min/max range can hold q, and for an exact lookup only
// those whose Bloom filter admits it. q must already be normalised.
func Lookup(r io.ReaderAt, size int64, q string, prefix bool, limit int) ([]Row, error) {
	f, err := parquet.OpenFile(r, size)
	if err != nil {
		return nil, err
	}
	var out []Row
	meta := f.Metadata().RowGroups
	for i, rg := range f.RowGroups() {
		// The chunk statistics live in the footer, so ruling a group out costs no extra read.
		st := meta[i].Columns[0].MetaData.Statistics
		lo, hi := string(st.MinValue), string(st.MaxValue)
		if hi < q || (lo > q && !strings.HasPrefix(lo, q)) { // [lo, hi] cannot hold q or anything starting with q
			continue
		}
		ch := rg.ColumnChunks()[0] // norm
		if !prefix {
			if bf := ch.BloomFilter(); bf != nil {
				if hit, err := bf.Check(parquet.ByteArrayValue([]byte(q))); err != nil {
					return nil, err
				} else if !hit {
					continue
				}
			}
		}
		rows := rg.Rows()
		buf := make([]parquet.Row, 64)
	read:
		for {
			n, err := rows.ReadRows(buf)
			for _, row := range buf[:n] {
				v := Row{Norm: string(row[0].ByteArray()), Name: string(row[1].ByteArray()), Table: string(row[2].ByteArray()), ID: row[3].Int32()}
				switch {
				case v.Norm == q || prefix && strings.HasPrefix(v.Norm, q):
					out = append(out, v)
					if len(out) >= limit {
						rows.Close()
						return out, nil
					}
				case v.Norm > q && !strings.HasPrefix(v.Norm, q):
					break read
				}
			}
			if err != nil {
				break
			}
		}
		rows.Close()
	}
	return out, nil
}

// ReadAll returns every row of a names.parquet file's bytes, in file order.
func ReadAll(b []byte) ([]Row, error) {
	return parquet.Read[Row](bytes.NewReader(b), int64(len(b)))
}

// IsEnglish reports whether a name could be an English one: no letter outside basic Latin,
// so no accented Latin letters (é, ô, ß) and no Cyrillic, Greek, CJK or other scripts.
// Digits, spaces and punctuation, curly quotes included, pass. An empty name does not.
func IsEnglish(s string) bool {
	if strings.TrimSpace(s) == "" {
		return false
	}
	for _, r := range s {
		if unicode.IsLetter(r) && r > unicode.MaxASCII {
			return false
		}
	}
	return true
}
