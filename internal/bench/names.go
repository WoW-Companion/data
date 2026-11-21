package bench

import (
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/WoW-Companion/data/internal/names"
)

// PrefixLen is the length of the prefix the names bench searches for, and PrefixLimit the most
// rows it asks for.
const (
	PrefixLen   = 4
	PrefixLimit = 20
)

// NameCosts is the cost of lookups in names.parquet by names.Lookup, each over a freshly opened
// file: an exact name, and a prefix of PrefixLen characters.
type NameCosts struct {
	Rows, Size   int64
	Exact        Case // Local and HTTP only; the cold fields are the measurements
	Prefix       Case
	PrefixRowsAv float64 // rows a prefix lookup returns, mean
}

// Names measures names.Lookup on dir's names.parquet, locally and over HTTP.
func Names(dir string, cfg Config) (NameCosts, error) {
	path := filepath.Join(dir, names.File)
	st, err := os.Stat(path)
	if err != nil {
		return NameCosts{}, err
	}
	all, err := readNames(path)
	if err != nil {
		return NameCosts{}, err
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(cfg.Latency)
		http.ServeFile(w, r, path)
	}))
	defer srv.Close()
	nc := NameCosts{Rows: int64(len(all)), Size: st.Size()}
	if len(all) == 0 {
		return nc, nil
	}
	for _, c := range []struct {
		dst    *Case
		prefix bool
	}{{&nc.Exact, false}, {&nc.Prefix, true}} {
		for _, side := range []struct {
			dst *Stat
			n   int
			url string
		}{{&c.dst.LocalCold, cfg.Local, ""}, {&c.dst.HTTPCold, cfg.HTTP, srv.URL + "/" + names.File}} {
			rng := rand.New(rand.NewSource(1))
			var times []float64
			var bytes, reqs, rows int64
			for i := 0; i < side.n; i++ {
				q := all[rng.Intn(len(all))]
				if c.prefix {
					q = q[:min(len(q), PrefixLen)]
				}
				cr, err := openCounted(path, side.url)
				if err != nil {
					return nc, err
				}
				start := time.Now()
				got, err := names.Lookup(cr, st.Size(), q, c.prefix, PrefixLimit)
				if err != nil {
					return nc, err
				}
				if len(got) == 0 {
					return nc, fmt.Errorf("lookup %q found nothing", q)
				}
				times = append(times, float64(time.Since(start).Microseconds())/1000)
				bytes += cr.bytes.Load()
				reqs += cr.reqs.Load()
				rows += int64(len(got))
			}
			if side.n == 0 {
				continue
			}
			n := int64(side.n)
			*side.dst = Stat{P50: pct(times, 0.50), P95: pct(times, 0.95), Bytes: bytes / n, Reqs: float64(reqs) / float64(n)}
			if c.prefix && side.url == "" {
				nc.PrefixRowsAv = float64(rows) / float64(n)
			}
		}
	}
	return nc, nil
}

func openCounted(path, url string) (*countReader, error) {
	if url != "" {
		return &countReader{r: rangeReader{url, &http.Client{}}}, nil
	}
	fh, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	return &countReader{r: fh}, nil // the handle lives until process exit
}

// readNames returns the norm of every row of names.parquet, in file order.
func readNames(path string) ([]string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	rows, err := names.ReadAll(b)
	if err != nil {
		return nil, err
	}
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.Norm
	}
	return out, nil
}

// NamesMarkdown renders the names costs.
func NamesMarkdown(nc NameCosts, cfg Config) string {
	var b strings.Builder
	fmt.Fprintf(&b, "\n### Name lookups in %s (cold, file opened per lookup; ms p50 / p95)\n\n%d rows, %d bytes. Exact: the whole normalised name. Prefix: the first %d characters, at most %d rows (mean %.1f returned).\n\n",
		names.File, nc.Rows, nc.Size, PrefixLen, PrefixLimit, nc.PrefixRowsAv)
	b.WriteString("| lookup | local | http | local bytes | http bytes | http reqs |\n|---|---|---|---|---|---|\n")
	for _, c := range []struct {
		n string
		c Case
	}{{"exact", nc.Exact}, {"prefix", nc.Prefix}} {
		fmt.Fprintf(&b, "| %s | %s | %s | %d | %d | %.1f |\n", c.n, ms(c.c.LocalCold), ms(c.c.HTTPCold), c.c.LocalCold.Bytes, c.c.HTTPCold.Bytes, c.c.HTTPCold.Reqs)
	}
	return b.String()
}
