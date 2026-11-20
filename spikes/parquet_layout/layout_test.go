// Package parquet_layout measures Parquet layouts for the real items and quests tables
// with parquet-go, the reader the advisor uses.
//
// Run after `make fetch`:
//
//	make spike
//
// It builds each layout from the cached client tables, then times point lookups and one
// range query per table from a local file and over HTTP range requests (an httptest
// server, so request counts and bytes are real but latency is loopback). Results print
// as markdown and are written to dist/spike/results.md. Takes well under two minutes.
package parquet_layout

import (
	"bytes"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/WoW-Companion/data/internal/fetch"
	"github.com/WoW-Companion/data/internal/manifest"
	"github.com/WoW-Companion/data/internal/parquetio"
	"github.com/WoW-Companion/data/internal/tables"
	"github.com/parquet-go/parquet-go"
)

var rowGroups = []int{4096, 16384, 65536}

const (
	lookupsWarm     = 1000
	lookupsCold     = 50
	lookupsHTTPWarm = 200
	lookupsHTTPCold = 10
	lookupsNames    = 100
	rangeRuns       = 10
	seed            = 7
)

// variant is one table written with one layout.
type variant struct {
	table string
	sort  string
	rg    int
	bloom bool
	path  string
	size  int64
	data  []byte
}

func (v variant) label() string {
	return fmt.Sprintf("%s rg=%dk bloom=%v sort=%s", v.table, v.rg/1024, v.bloom, v.sort)
}

// ---------------------------------------------------------------- measuring plumbing

type counted struct {
	reqs, bytes atomic.Int64
}

type countingWriter struct {
	http.ResponseWriter
	c *counted
}

func (w countingWriter) Write(b []byte) (int, error) {
	w.c.bytes.Add(int64(len(b)))
	return w.ResponseWriter.Write(b)
}

// rangeReader reads a remote file with one HTTP Range request per ReadAt.
type rangeReader struct {
	url    string
	client *http.Client
}

func (r rangeReader) ReadAt(p []byte, off int64) (int, error) {
	req, _ := http.NewRequest(http.MethodGet, r.url, nil)
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", off, off+int64(len(p))-1))
	resp, err := r.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	return io.ReadFull(resp.Body, p)
}

// source opens the same variant locally or over HTTP.
type source struct {
	name string // "local" or "http"
	open func() (*parquet.File, error)
}

func localSource(v variant) source {
	return source{"local", func() (*parquet.File, error) {
		f, err := os.Open(v.path)
		if err != nil {
			return nil, err
		}
		return parquet.OpenFile(f, v.size) // closed at process exit
	}}
}

func httpSource(base string, v variant) source {
	client := &http.Client{}
	return source{"http", func() (*parquet.File, error) {
		return parquet.OpenFile(rangeReader{base + "/" + filepath.Base(v.path), client}, v.size)
	}}
}

func pct(xs []float64, p float64) float64 {
	s := slices.Clone(xs)
	slices.Sort(s)
	return s[min(len(s)-1, int(p*float64(len(s))))]
}

func ms(d time.Duration) float64 { return float64(d.Microseconds()) / 1000 }

// peakHeap runs f and returns the highest HeapAlloc (MB) sampled while it ran.
func peakHeap(f func()) float64 {
	runtime.GC()
	var peak atomic.Uint64
	stop, done := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(done)
		var m runtime.MemStats
		for {
			runtime.ReadMemStats(&m)
			if m.HeapAlloc > peak.Load() {
				peak.Store(m.HeapAlloc)
			}
			select {
			case <-stop:
				return
			case <-time.After(2 * time.Millisecond):
			}
		}
	}()
	f()
	close(stop)
	<-done
	return float64(peak.Load()) / (1 << 20)
}

// ---------------------------------------------------------------- queries

func col(f *parquet.File, name string) int {
	c, ok := f.Schema().Lookup(name)
	if !ok {
		panic("no column " + name)
	}
	return c.ColumnIndex
}

// groupBounds returns the min and max of an int32 column chunk from its page index.
func groupBounds(ch parquet.ColumnChunk) (lo, hi int32, ok bool) {
	ci, err := ch.ColumnIndex()
	if err != nil || ci.NumPages() == 0 {
		return 0, 0, false
	}
	first := true
	for i := 0; i < ci.NumPages(); i++ {
		if ci.NullPage(i) {
			continue
		}
		a, b := ci.MinValue(i).Int32(), ci.MaxValue(i).Int32()
		if first || a < lo {
			lo = a
		}
		if first || b > hi {
			hi = b
		}
		first = false
	}
	return lo, hi, !first
}

// fetchRow reads one full row of group rg.
func fetchRow[T any](rg parquet.RowGroup, row int64) (T, error) {
	r := parquet.NewGenericRowGroupReader[T](rg)
	defer r.Close()
	var out T
	if err := r.SeekToRow(row); err != nil {
		return out, err
	}
	buf := make([]T, 1)
	if _, err := r.Read(buf); err != nil && err != io.EOF {
		return out, err
	}
	return buf[0], nil
}

// lookupID finds one id: prune groups by statistics, then by Bloom filter when the file
// has one, then read just the page of the id column that can hold it, then the row.
func lookupID[T any](f *parquet.File, id int32) (bool, error) {
	idc := col(f, "id")
	for _, rg := range f.RowGroups() {
		ch := rg.ColumnChunks()[idc]
		lo, hi, ok := groupBounds(ch)
		if !ok || id < lo || id > hi {
			continue
		}
		if bf := ch.BloomFilter(); bf != nil {
			if hit, err := bf.Check(parquet.ValueOf(id)); err != nil || !hit {
				continue
			}
		}
		ci, _ := ch.ColumnIndex()
		oi, err := ch.OffsetIndex()
		if err != nil {
			return false, err
		}
		for p := 0; p < ci.NumPages(); p++ {
			if id < ci.MinValue(p).Int32() || id > ci.MaxValue(p).Int32() {
				continue
			}
			pages := ch.Pages()
			if err := pages.SeekToRow(oi.FirstRowIndex(p)); err != nil {
				return false, err
			}
			page, err := pages.ReadPage()
			pages.Close()
			if err != nil {
				return false, err
			}
			vals := make([]parquet.Value, page.NumValues())
			n, _ := page.Values().ReadValues(vals)
			for i, v := range vals[:n] {
				if v.Int32() == id {
					_, err := fetchRow[T](rg, oi.FirstRowIndex(p)+int64(i))
					return err == nil, err
				}
			}
		}
	}
	return false, nil
}

// lookupName finds an item by exact name. The column is not sorted, so without a Bloom
// filter every group's name column is read.
func lookupName(f *parquet.File, name string) (bool, error) {
	nc := col(f, "name")
	for _, rg := range f.RowGroups() {
		ch := rg.ColumnChunks()[nc]
		if bf := ch.BloomFilter(); bf != nil {
			if hit, err := bf.Check(parquet.ValueOf(name)); err != nil || !hit {
				continue
			}
		}
		pages := ch.Pages()
		var row int64
		for {
			page, err := pages.ReadPage()
			if err == io.EOF {
				break
			}
			if err != nil {
				pages.Close()
				return false, err
			}
			vals := make([]parquet.Value, page.NumValues())
			n, _ := page.Values().ReadValues(vals)
			for i, v := range vals[:n] {
				if string(v.ByteArray()) == name {
					pages.Close()
					_, err := fetchRow[tables.Item](rg, row+int64(i))
					return err == nil, err
				}
			}
			row += int64(n)
		}
		pages.Close()
	}
	return false, nil
}

// rangeQuery counts rows matching int32 equality/range predicates, reading only the
// groups whose statistics can match. Returns count and groups kept.
type pred struct {
	col    string
	lo, hi int32
}

func rangeQuery[T any](f *parquet.File, preds []pred, match func(T) bool) (count, kept int, err error) {
	for _, rg := range f.RowGroups() {
		skip := false
		for _, p := range preds {
			lo, hi, ok := groupBounds(rg.ColumnChunks()[col(f, p.col)])
			if ok && (hi < p.lo || lo > p.hi) {
				skip = true
			}
		}
		if skip {
			continue
		}
		kept++
		r := parquet.NewGenericRowGroupReader[T](rg)
		buf := make([]T, 1024)
		for {
			n, rerr := r.Read(buf)
			for _, row := range buf[:n] {
				if match(row) {
					count++
				}
			}
			if rerr != nil {
				break
			}
		}
		r.Close()
	}
	return count, kept, nil
}

// ---------------------------------------------------------------- harness

type report struct {
	sizes, points, names, ranges strings.Builder
}

func timeLookups(s source, n int, cold bool, do func(f *parquet.File, i int) error) (p50, p95 float64, err error) {
	var shared *parquet.File
	if !cold {
		if shared, err = s.open(); err != nil {
			return
		}
		if err = do(shared, 0); err != nil { // warm-up
			return
		}
	}
	var xs []float64
	for i := 0; i < n; i++ {
		start := time.Now()
		f := shared
		if cold {
			if f, err = s.open(); err != nil {
				return
			}
		}
		if err = do(f, i); err != nil {
			return
		}
		xs = append(xs, ms(time.Since(start)))
	}
	return pct(xs, .5), pct(xs, .95), nil
}

func BenchmarkLayout(b *testing.B) {
	cacheDir := filepath.Join("..", "..", "dist", "cache", fetch.DefaultBuild)
	if _, err := os.Stat(filepath.Join(cacheDir, fetch.ProvenanceFile)); err != nil {
		b.Skip("no cached client tables: run `make fetch` first")
	}
	b.StopTimer()
	runLayout(b, cacheDir)
}

func runLayout(tb testing.TB, cacheDir string) {
	fail := func(err error) {
		if err != nil {
			tb.Fatal(err)
		}
	}
	cache, err := tables.LoadCache(cacheDir)
	fail(err)
	src := tables.SourceFrom(cache.Prov)
	outDir := filepath.Join("..", "..", "dist", "spike")
	fail(os.MkdirAll(outDir, 0o755))

	// Server over the spike files, counting requests and response bytes.
	var ctr counted
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctr.reqs.Add(1)
		http.ServeFile(countingWriter{w, &ctr}, r, filepath.Join(outDir, filepath.Base(r.URL.Path)))
	}))
	defer srv.Close()

	var out strings.Builder
	w := func(format string, a ...any) { fmt.Fprintf(&out, format, a...) }

	// ---- build variants
	itemBloom, questBloom := []string{"id", "name"}, []string{"id"}
	var all []variant
	for _, rg := range rowGroups {
		for _, bl := range []bool{false, true} {
			ib, qb := []string(nil), []string(nil)
			if bl {
				ib, qb = itemBloom, questBloom
			}
			all = append(all, buildV(tb, outDir, "items", "by_id", cache.Items, rg, ib, src))
			all = append(all, buildV(tb, outDir, "quests", "by_id", cache.Quests, rg, qb, src))
		}
	}

	w("### File sizes (parquet-go, zstd, sorted by id)\n\n| table | row group (k rows) | bloom | row groups | bytes |\n|---|---|---|---|---|\n")
	for _, v := range all {
		f, err := parquet.OpenFile(bytes.NewReader(v.data), v.size)
		fail(err)
		w("| %s | %d | %s | %d | %d |\n", v.table, v.rg/1024, yn(v.bloom), len(f.RowGroups()), v.size)
	}

	// ---- point lookups by id
	rnd := rand.New(rand.NewSource(seed))
	itemIDs := make([]int32, len(cache.Items))
	for i, r := range cache.Items {
		itemIDs[i] = r.ID
	}
	questIDs := make([]int32, len(cache.Quests))
	for i, r := range cache.Quests {
		questIDs[i] = r.ID
	}
	pick := func(ids []int32, n int) []int32 {
		out := make([]int32, n)
		for i := range out {
			out[i] = ids[rnd.Intn(len(ids))]
		}
		return out
	}
	names := make([]string, 0, lookupsNames)
	for len(names) < lookupsNames {
		names = append(names, cache.Items[rnd.Intn(len(cache.Items))].Name)
	}

	w("\n### Point lookups by id (ms; heap = peak Go heap MB while the case ran)\n\n| table | rg (k) | bloom | where | mode | n | p50 | p95 | heap MB |\n|---|---|---|---|---|---|---|---|---|\n")
	for _, v := range all {
		ids := itemIDs
		if v.table == "quests" {
			ids = questIDs
		}
		for _, s := range []source{localSource(v), httpSource(srv.URL, v)} {
			for _, cold := range []bool{true, false} {
				n := map[[2]bool]int{{false, false}: lookupsWarm, {false, true}: lookupsCold, {true, false}: lookupsHTTPWarm, {true, true}: lookupsHTTPCold}[[2]bool{s.name == "http", cold}]
				targets := pick(ids, n)
				var p50, p95 float64
				heap := peakHeap(func() {
					p50, p95, err = timeLookups(s, n, cold, func(f *parquet.File, i int) error {
						var ok bool
						var err error
						if v.table == "items" {
							ok, err = lookupID[tables.Item](f, targets[i])
						} else {
							ok, err = lookupID[tables.Quest](f, targets[i])
						}
						if err == nil && !ok {
							err = fmt.Errorf("id %d not found in %s", targets[i], v.label())
						}
						return err
					})
				})
				fail(err)
				w("| %s | %d | %s | %s | %s | %d | %.2f | %.2f | %.1f |\n", v.table, v.rg/1024, yn(v.bloom), s.name, mode(cold), n, p50, p95, heap)
			}
		}
	}

	// ---- exact name lookups (items only)
	w("\n### Item lookups by exact name (warm, ms)\n\n| rg (k) | bloom | where | n | p50 | p95 | heap MB |\n|---|---|---|---|---|---|---|\n")
	for _, v := range all {
		if v.table != "items" {
			continue
		}
		for _, s := range []source{localSource(v), httpSource(srv.URL, v)} {
			var p50, p95 float64
			heap := peakHeap(func() {
				p50, p95, err = timeLookups(s, len(names), false, func(f *parquet.File, i int) error {
					ok, err := lookupName(f, names[i])
					if err == nil && !ok {
						err = fmt.Errorf("name %q not found", names[i])
					}
					return err
				})
			})
			fail(err)
			w("| %d | %s | %s | %d | %.2f | %.2f | %.1f |\n", v.rg/1024, yn(v.bloom), s.name, len(names), p50, p95, heap)
		}
	}

	// ---- range queries: sort variants at 4k and 16k rows per group
	w("\n### Range queries (items: slot 5, item level 40-60; quests: map 1, id 20000-60000)\n\n| table | rg (k) | sort | groups kept by stats | rows | where | p50 ms | p95 ms | heap MB | HTTP requests (all runs) | HTTP bytes (all runs) |\n|---|---|---|---|---|---|---|---|---|---|---|\n")
	for _, rg := range []int{4096, 16384} {
		rv := []variant{
			buildV(tb, outDir, "items", "by_id", cache.Items, rg, nil, src),
			buildV(tb, outDir, "items", "by_slot_level", sortedItems(cache.Items), rg, nil, src),
			buildV(tb, outDir, "quests", "by_id", cache.Quests, rg, nil, src),
			buildV(tb, outDir, "quests", "by_map_id", sortedQuests(cache.Quests), rg, nil, src),
		}
		for _, v := range rv {
			for _, s := range []source{localSource(v), httpSource(srv.URL, v)} {
				var xs []float64
				var count, kept int
				ctr.reqs.Store(0)
				ctr.bytes.Store(0)
				heap := peakHeap(func() {
					for i := 0; i <= rangeRuns; i++ { // run 0 is the warm-up and is not timed
						f, oerr := s.open()
						fail(oerr)
						start := time.Now()
						if v.table == "items" {
							count, kept, err = rangeQuery(f, []pred{{"slot", 5, 5}, {"item_level", 40, 60}}, func(r tables.Item) bool { return r.Slot == 5 && r.ItemLevel >= 40 && r.ItemLevel <= 60 })
						} else {
							count, kept, err = rangeQuery(f, []pred{{"map_id", 1, 1}, {"id", 20000, 60000}}, func(r tables.Quest) bool { return r.MapID != nil && *r.MapID == 1 && r.ID >= 20000 && r.ID <= 60000 })
						}
						fail(err)
						if i > 0 {
							xs = append(xs, ms(time.Since(start)))
						}
					}
				})
				httpReq, httpBytes := "", ""
				if s.name == "http" {
					httpReq, httpBytes = fmt.Sprint(ctr.reqs.Load()), fmt.Sprint(ctr.bytes.Load())
				}
				f, _ := s.open()
				w("| %s | %d | %s | %d/%d | %d | %s | %.2f | %.2f | %.1f | %s | %s |\n", v.table, rg/1024, v.sort, kept, len(f.RowGroups()), count, s.name, pct(xs, .5), pct(xs, .95), heap, httpReq, httpBytes)
			}
		}
	}

	fmt.Print(out.String())
	fail(os.WriteFile(filepath.Join(outDir, "results.md"), []byte(out.String()), 0o644))
}

func buildV[T any](tb testing.TB, dir, table, sortName string, rows []T, rg int, bloom []string, src manifest.Source) variant {
	opts := parquetio.Options{RowGroup: rg, Bloom: bloom, Sources: []manifest.Source{src}}
	if sortName == "by_id" {
		opts.Sort = []string{"id"}
	}
	b, err := parquetio.Encode(rows, opts)
	if err != nil {
		tb.Fatal(err)
	}
	v := variant{table: table, sort: sortName, rg: rg, bloom: len(bloom) > 0, size: int64(len(b)), data: b}
	v.path = filepath.Join(dir, fmt.Sprintf("%s_%s_%dk_bloom%v.parquet", table, sortName, rg/1024, v.bloom))
	if err := os.WriteFile(v.path, b, 0o644); err != nil {
		tb.Fatal(err)
	}
	return v
}

func sortedItems(in []tables.Item) []tables.Item {
	out := slices.Clone(in)
	slices.SortFunc(out, func(a, b tables.Item) int {
		if a.Slot != b.Slot {
			return int(a.Slot) - int(b.Slot)
		}
		if a.ItemLevel != b.ItemLevel {
			return int(a.ItemLevel) - int(b.ItemLevel)
		}
		return int(a.ID) - int(b.ID)
	})
	return out
}

// sortedQuests orders by (map_id, id) with quests that have no map last.
func sortedQuests(in []tables.Quest) []tables.Quest {
	out := slices.Clone(in)
	key := func(q tables.Quest) int32 {
		if q.MapID == nil {
			return 1 << 30
		}
		return *q.MapID
	}
	slices.SortFunc(out, func(a, b tables.Quest) int {
		if key(a) != key(b) {
			return int(key(a)) - int(key(b))
		}
		return int(a.ID) - int(b.ID)
	})
	return out
}

func yn(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

func mode(cold bool) string {
	if cold {
		return "cold"
	}
	return "warm"
}
