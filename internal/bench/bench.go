// Package bench measures a built dataset: per table its size, layout and how fast and how
// many bytes a point lookup by the leading sort key and a 100-row range read take, from the
// local disk and over HTTP range requests with injected latency (object storage).
//
// "cold" opens the file afresh for every lookup, so the footer and page index are read each
// time, as a stateless client would; "warm" reuses one open file. The OS page cache is not
// dropped, so local cold understates a cold disk.
package bench

import (
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/parquet-go/parquet-go"
	"github.com/parquet-go/parquet-go/format"
)

// RangeRows is the size of the range read: this many consecutive keys.
const RangeRows = 100

// Config sets how many lookups run and how slow the simulated object store is.
type Config struct {
	Local   int           // lookups per local case
	HTTP    int           // lookups per HTTP case
	Latency time.Duration // delay added to every HTTP request
}

// Stat is the timing of one case. Bytes and Reqs are per lookup (means).
type Stat struct {
	P50, P95 float64 // milliseconds
	Bytes    int64
	Reqs     float64
}

// Case holds one operation's four measurements.
type Case struct {
	LocalCold, LocalWarm, HTTPCold, HTTPWarm Stat
}

// Table is one table's result.
type Table struct {
	Name      string
	Rows      int64
	Size      int64
	RowGroups int
	Key       string   // leading sort key, the column looked up
	Bloom     []string // columns carrying a Bloom filter
	BloomSize int64    // bytes of all Bloom filters
	DictCols  int      // columns written with dictionary encoding
	Cols      int
	Point     Case
	Range     Case
}

// countReader counts the bytes and requests of a ReaderAt.
type countReader struct {
	r           io.ReaderAt
	bytes, reqs atomic.Int64
}

func (c *countReader) ReadAt(p []byte, off int64) (int, error) {
	c.reqs.Add(1)
	n, err := c.r.ReadAt(p, off)
	c.bytes.Add(int64(n))
	return n, err
}

// rangeReader reads a remote file with one HTTP Range request per ReadAt.
type rangeReader struct {
	url    string
	client *http.Client
}

func (r rangeReader) ReadAt(p []byte, off int64) (int, error) {
	req, err := http.NewRequest(http.MethodGet, r.url, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", off, off+int64(len(p))-1))
	resp, err := r.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	return io.ReadFull(resp.Body, p)
}

// target is one file opened one way.
type target struct {
	open func() (*countReader, *parquet.File, error)
}

// Run benchmarks every .parquet file in dir.
func Run(dir string, cfg Config) ([]Table, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.parquet"))
	if err != nil || len(paths) == 0 {
		return nil, fmt.Errorf("bench: no .parquet files in %s", dir)
	}
	sort.Strings(paths)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(cfg.Latency)
		http.ServeFile(w, r, filepath.Join(dir, filepath.Base(r.URL.Path)))
	}))
	defer srv.Close()
	var out []Table
	for _, p := range paths {
		t, err := runTable(p, srv.URL, cfg)
		if err != nil {
			return nil, fmt.Errorf("bench %s: %w", filepath.Base(p), err)
		}
		out = append(out, t)
	}
	return out, nil
}

func runTable(path, base string, cfg Config) (Table, error) {
	st, err := os.Stat(path)
	if err != nil {
		return Table{}, err
	}
	local := target{func() (*countReader, *parquet.File, error) {
		fh, err := os.Open(path)
		if err != nil {
			return nil, nil, err
		}
		c := &countReader{r: fh} // the handle lives until process exit
		f, err := parquet.OpenFile(c, st.Size())
		return c, f, err
	}}
	remote := target{func() (*countReader, *parquet.File, error) {
		c := &countReader{r: rangeReader{base + "/" + filepath.Base(path), &http.Client{}}}
		f, err := parquet.OpenFile(c, st.Size())
		return c, f, err
	}}
	_, f, err := local.open()
	if err != nil {
		return Table{}, err
	}
	t := describe(f, filepath.Base(path), st.Size())
	if t.Key == "" {
		return t, fmt.Errorf("no sorting column")
	}
	keys, err := readKeys(f, t.Key)
	if err != nil {
		return t, err
	}
	if len(keys) == 0 {
		return t, nil
	}
	rng := rand.New(rand.NewSource(1))
	pick := func(n int) []int {
		idx := make([]int, n)
		for i := range idx {
			idx[i] = rng.Intn(len(keys))
		}
		return idx
	}
	lidx, hidx := pick(cfg.Local), pick(cfg.HTTP)
	run := func(tg target, idx []int, cold bool, op func(*parquet.File, []parquet.Value, int) error) (Stat, error) {
		return measure(tg, idx, cold, func(f *parquet.File, i int) error { return op(f, keys, i) })
	}
	for _, c := range []struct {
		dst *Case
		op  func(*parquet.File, []parquet.Value, int) error
	}{{&t.Point, pointOp(t.Key)}, {&t.Range, rangeOp(t.Key)}} {
		if c.dst.LocalCold, err = run(local, lidx, true, c.op); err != nil {
			return t, err
		}
		if c.dst.LocalWarm, err = run(local, lidx, false, c.op); err != nil {
			return t, err
		}
		if c.dst.HTTPCold, err = run(remote, hidx, true, c.op); err != nil {
			return t, err
		}
		if c.dst.HTTPWarm, err = run(remote, hidx, false, c.op); err != nil {
			return t, err
		}
	}
	return t, nil
}

// describe reads the layout facts out of the file's footer.
func describe(f *parquet.File, name string, size int64) Table {
	t := Table{Name: name, Rows: f.NumRows(), Size: size, RowGroups: len(f.RowGroups())}
	cols := f.Schema().Columns()
	t.Cols = len(cols)
	if rgs := f.RowGroups(); len(rgs) > 0 {
		if sc := rgs[0].SortingColumns(); len(sc) > 0 {
			t.Key = strings.Join(sc[0].Path(), ".")
		}
		for i, ch := range rgs[0].ColumnChunks() {
			if ch.BloomFilter() != nil {
				t.Bloom = append(t.Bloom, strings.Join(cols[i], "."))
			}
		}
		for _, rg := range rgs {
			for _, ch := range rg.ColumnChunks() {
				if bf := ch.BloomFilter(); bf != nil {
					t.BloomSize += bf.Size()
				}
			}
		}
	}
	if md := f.Metadata(); len(md.RowGroups) > 0 {
		for _, c := range md.RowGroups[0].Columns {
			if slices.Contains(c.MetaData.Encoding, format.RLEDictionary) || slices.Contains(c.MetaData.Encoding, format.PlainDictionary) {
				t.DictCols++
			}
		}
	}
	return t
}

// leaf returns the column index and type of a top-level leaf column.
func leaf(f *parquet.File, name string) (int, parquet.Type, error) {
	c, ok := f.Schema().Lookup(name)
	if !ok {
		return 0, nil, fmt.Errorf("no column %q", name)
	}
	return c.ColumnIndex, c.Node.Type(), nil
}

// readKeys returns the key column of the whole file, in row order.
func readKeys(f *parquet.File, key string) ([]parquet.Value, error) {
	ki, _, err := leaf(f, key)
	if err != nil {
		return nil, err
	}
	var keys []parquet.Value
	for _, rg := range f.RowGroups() {
		pages := rg.ColumnChunks()[ki].Pages()
		for {
			pg, err := pages.ReadPage()
			if err == io.EOF {
				break
			}
			if err != nil {
				pages.Close()
				return nil, err
			}
			vals := make([]parquet.Value, pg.NumValues())
			n, _ := pg.Values().ReadValues(vals)
			for _, v := range vals[:n] {
				keys = append(keys, v.Clone())
			}
			parquet.Release(pg)
		}
		pages.Close()
	}
	return keys, nil
}

// measure times op over idx. Cold opens the file for each lookup; warm opens once.
func measure(tg target, idx []int, cold bool, op func(*parquet.File, int) error) (Stat, error) {
	var times []float64
	var bytes, reqs int64
	var cr *countReader
	var f *parquet.File
	var err error
	if !cold {
		if cr, f, err = tg.open(); err != nil {
			return Stat{}, err
		}
		if err = op(f, idx[0]); err != nil { // one throwaway lookup warms the file's lazy state
			return Stat{}, err
		}
		cr.bytes.Store(0)
		cr.reqs.Store(0)
	}
	for _, i := range idx {
		start := time.Now()
		if cold {
			if cr, f, err = tg.open(); err != nil {
				return Stat{}, err
			}
		}
		if err = op(f, i); err != nil {
			return Stat{}, err
		}
		times = append(times, float64(time.Since(start).Microseconds())/1000)
		if cold {
			bytes += cr.bytes.Load()
			reqs += cr.reqs.Load()
		}
	}
	if !cold {
		bytes, reqs = cr.bytes.Load(), cr.reqs.Load()
	}
	n := int64(len(idx))
	return Stat{P50: pct(times, 0.50), P95: pct(times, 0.95), Bytes: bytes / n, Reqs: float64(reqs) / float64(n)}, nil
}

func pct(xs []float64, p float64) float64 {
	s := slices.Clone(xs)
	slices.Sort(s)
	return s[min(len(s)-1, int(p*float64(len(s))))]
}

// span is a column chunk's pages with their key bounds.
type span struct {
	rg     parquet.RowGroup
	ch     parquet.ColumnChunk
	ci     parquet.ColumnIndex
	oi     parquet.OffsetIndex
	lo, hi parquet.Value
}

func spans(f *parquet.File, ki int) ([]span, error) {
	var out []span
	for _, rg := range f.RowGroups() {
		ch := rg.ColumnChunks()[ki]
		ci, err := ch.ColumnIndex()
		if err != nil {
			return nil, err
		}
		oi, err := ch.OffsetIndex()
		if err != nil {
			return nil, err
		}
		if ci.NumPages() == 0 {
			continue
		}
		out = append(out, span{rg, ch, ci, oi, ci.MinValue(0), ci.MaxValue(ci.NumPages() - 1)})
	}
	return out, nil
}

// readRows reads rows of rg from the row `from`, up to max of them, stopping early when stop says so.
func readRows(rg parquet.RowGroup, from int64, max int, ki int, stop func(parquet.Value) bool) (int, error) {
	rows := rg.Rows()
	defer rows.Close()
	if err := rows.SeekToRow(from); err != nil {
		return 0, err
	}
	buf := make([]parquet.Row, 1)
	n := 0
	for n < max {
		r, err := rows.ReadRows(buf)
		if r == 1 {
			var k parquet.Value
			for _, v := range buf[0] {
				if v.Column() == ki {
					k = v
					break
				}
			}
			if stop != nil && stop(k) {
				return n, nil
			}
			n++
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return n, err
		}
	}
	return n, nil
}

// pointOp looks up keys[i]: prune row groups by the page index and Bloom filter, read the one
// page that can hold the key, then read the row.
func pointOp(key string) func(*parquet.File, []parquet.Value, int) error {
	return func(f *parquet.File, keys []parquet.Value, i int) error {
		ki, typ, err := leaf(f, key)
		if err != nil {
			return err
		}
		want := keys[i]
		cmp := typ.Compare
		sps, err := spans(f, ki)
		if err != nil {
			return err
		}
		for _, s := range sps {
			if cmp(want, s.lo) < 0 || cmp(want, s.hi) > 0 {
				continue
			}
			if bf := s.ch.BloomFilter(); bf != nil {
				if hit, err := bf.Check(want); err != nil || !hit {
					continue
				}
			}
			for p := 0; p < s.ci.NumPages(); p++ {
				if cmp(want, s.ci.MinValue(p)) < 0 || cmp(want, s.ci.MaxValue(p)) > 0 {
					continue
				}
				pages := s.ch.Pages()
				if err := pages.SeekToRow(s.oi.FirstRowIndex(p)); err != nil {
					return err
				}
				pg, err := pages.ReadPage()
				pages.Close()
				if err != nil {
					return err
				}
				vals := make([]parquet.Value, pg.NumValues())
				n, _ := pg.Values().ReadValues(vals)
				for j, v := range vals[:n] {
					if cmp(v, want) == 0 {
						_, err := readRows(s.rg, s.oi.FirstRowIndex(p)+int64(j), 1, ki, nil)
						return err
					}
				}
			}
		}
		return fmt.Errorf("key %v not found", want)
	}
}

// rangeOp reads the rows whose key lies in [keys[i], keys[i+RangeRows-1]]: RangeRows consecutive
// rows of the sorted file (more where keys repeat).
func rangeOp(key string) func(*parquet.File, []parquet.Value, int) error {
	return func(f *parquet.File, keys []parquet.Value, i int) error {
		ki, typ, err := leaf(f, key)
		if err != nil {
			return err
		}
		i = min(i, max(0, len(keys)-RangeRows))
		lo, hi := keys[i], keys[min(len(keys)-1, i+RangeRows-1)]
		cmp := typ.Compare
		sps, err := spans(f, ki)
		if err != nil {
			return err
		}
		got := 0
		for _, s := range sps {
			if cmp(hi, s.lo) < 0 || cmp(lo, s.hi) > 0 {
				continue
			}
			first := 0
			for first < s.ci.NumPages()-1 && cmp(s.ci.MaxValue(first), lo) < 0 {
				first++
			}
			n, err := readRows(s.rg, s.oi.FirstRowIndex(first), int(s.rg.NumRows()), ki, func(k parquet.Value) bool { return cmp(k, hi) > 0 })
			if err != nil {
				return err
			}
			got += n
		}
		if want := min(RangeRows, len(keys)); got < want {
			return fmt.Errorf("range read %d rows, want at least %d", got, want)
		}
		return nil
	}
}
