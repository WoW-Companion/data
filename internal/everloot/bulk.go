package everloot

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// Everloot's data files export (https://everloot.app/docs/api/data-files): one NDJSON file
// per dataset in an immutable folder, named by latest.json and checked by index.json.
const (
	BulkBase = "https://data.everloot.app"
	// WikiCredit is owed wherever item, quest, NPC or spell names come from the export.
	WikiCredit = "Names from the Warcraft Wiki (https://warcraft.wiki.gg), CC BY-SA 4.0."
	// OriginAPI and OriginCarried are the manifest wording of the other two routes.
	OriginAPI = "api"
	exportDir = "export"
	maxFile   = 1 << 30
)

//go:embed schema/bulkIndex.json
var bulkIndexSchema []byte

// IndexFile is one files[] entry of index.json.
type IndexFile struct {
	Name    string `json:"name"`
	Dataset string `json:"dataset"`
	Field   string `json:"field"`
	Rows    int64  `json:"rows"`
	Bytes   int64  `json:"bytes"`
	SHA256  string `json:"sha256"`
}

// Index is the part of index.json the pipeline reads.
type Index struct {
	GeneratedAt string      `json:"generatedAt"`
	DataAsOf    string      `json:"dataAsOf"`
	Build       string      `json:"build"`
	Files       []IndexFile `json:"files"`
}

// Export is one verified export folder in the cache.
type Export struct {
	Path  string // the index path relative to the base, e.g. 2026-10-19T06-40-12-328Z/index.json
	Dir   string // the cached folder holding the verified files
	Index Index
}

// Origin is the manifest wording of a source read from this export.
func (e *Export) Origin() string { return "export " + e.Path }

// file returns the index entry of a dataset's file with that field, if the export has rows for it.
func (e *Export) file(dataset, field string) (IndexFile, bool) {
	for _, f := range e.Index.Files {
		if f.Dataset == dataset && f.Field == field && f.Rows > 0 {
			return f, true
		}
	}
	return IndexFile{}, false
}

// ValidateIndex checks an index.json body against the bulkIndex schema.
func ValidateIndex(body []byte) error {
	var schema, doc any
	if err := json.Unmarshal(bulkIndexSchema, &schema); err != nil {
		return err
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return fmt.Errorf("index.json: %w", err)
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource("https://everloot.app/schema/api/bulkIndex.json", schema); err != nil {
		return err
	}
	s, err := c.Compile("https://everloot.app/schema/api/bulkIndex.json")
	if err != nil {
		return err
	}
	if err := s.Validate(doc); err != nil {
		return fmt.Errorf("index.json does not match bulkIndex: %w", err)
	}
	return nil
}

func bulkClient() *http.Client { return &http.Client{Timeout: 10 * time.Minute} }

// httpBody fetches a small document (no API key is ever sent to the files host).
func httpBody(u string) ([]byte, error) {
	resp, err := bulkClient().Get(u)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: HTTP %d", u, resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 64<<20))
}

// safeRel reports whether p is a plain relative path with no way out of the base.
func safeRel(p string) bool {
	return p != "" && !strings.HasPrefix(p, "/") && !strings.Contains(p, "\\") && !strings.Contains(p, "..") && path.Clean(p) == p
}

// fileSum is the SHA-256 of the file at p, "" when unreadable.
func fileSum(p string) string {
	f, err := os.Open(p)
	if err != nil {
		return ""
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return ""
	}
	return hex.EncodeToString(h.Sum(nil))
}

// download writes u to dst through a temp file and keeps it only when its SHA-256 is want.
func download(u, dst, want string) error {
	resp, err := bulkClient().Get(u)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: HTTP %d", u, resp.StatusCode)
	}
	tmp, err := os.CreateTemp(filepath.Dir(dst), ".dl-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	h := sha256.New()
	_, err = io.Copy(io.MultiWriter(tmp, h), io.LimitReader(resp.Body, maxFile))
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != strings.ToLower(want) {
		return fmt.Errorf("%s: checksum mismatch: index says %s, file is %s", u, want, got)
	}
	return os.Rename(tmp.Name(), dst)
}

// wanted are the datasets the pipeline reads from an export.
var wanted = []struct{ dataset, field string }{{"drops", "rates"}, {"edges", "edges"}, {"corpus", "records"}}

// FetchExport reads latest.json and the index it names, validates the index, and caches
// each wanted file under cacheDir/export/<folder>/ after checking its SHA-256. A cached
// file whose checksum matches is not downloaded again: an export folder never changes.
func FetchExport(bulkBase, cacheDir string) (*Export, error) {
	bulkBase = strings.TrimRight(bulkBase, "/")
	body, err := httpBody(bulkBase + "/latest.json")
	if err != nil {
		return nil, fmt.Errorf("latest.json: %w", err)
	}
	var latest struct {
		Index string `json:"index"`
	}
	if err := json.Unmarshal(body, &latest); err != nil || !safeRel(latest.Index) {
		return nil, fmt.Errorf("latest.json: no usable index path")
	}
	// A folder's index is as immutable as its files: read the cached copy when there is one.
	dir := filepath.Join(cacheDir, exportDir, filepath.FromSlash(path.Dir(latest.Index)))
	idxPath := filepath.Join(dir, path.Base(latest.Index))
	body, err = os.ReadFile(idxPath)
	if err != nil || ValidateIndex(body) != nil {
		if body, err = httpBody(bulkBase + "/" + latest.Index); err != nil {
			return nil, fmt.Errorf("index: %w", err)
		}
		if err := ValidateIndex(body); err != nil {
			return nil, err
		}
	}
	var ix Index
	if err := json.Unmarshal(body, &ix); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(idxPath, body, 0o644); err != nil {
		return nil, err
	}
	ex := &Export{Path: latest.Index, Dir: dir, Index: ix}
	for _, w := range wanted {
		f, ok := ex.file(w.dataset, w.field)
		if !ok {
			continue
		}
		if !safeRel(f.Name) || strings.Contains(f.Name, "/") {
			return nil, fmt.Errorf("index names an unusable file %q", f.Name)
		}
		dst := filepath.Join(dir, f.Name)
		if fileSum(dst) == strings.ToLower(f.SHA256) {
			continue
		}
		if err := download(bulkBase+"/"+path.Dir(latest.Index)+"/"+f.Name, dst, f.SHA256); err != nil {
			return nil, err
		}
	}
	return ex, nil
}

// readNDJSON streams the lines of one verified file through fn.
func (e *Export) readNDJSON(f IndexFile, fn func(line []byte) error) error {
	fh, err := os.Open(filepath.Join(e.Dir, f.Name))
	if err != nil {
		return err
	}
	defer fh.Close()
	sc := bufio.NewScanner(fh)
	sc.Buffer(make([]byte, 1<<20), 64<<20)
	var n int64
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		if err := fn(line); err != nil {
			return fmt.Errorf("%s line %d: %w", f.Name, n+1, err)
		}
		n++
	}
	if err := sc.Err(); err != nil {
		return err
	}
	if n != f.Rows {
		return fmt.Errorf("%s: %d rows, index says %d", f.Name, n, f.Rows)
	}
	return nil
}

// Rates streams the export's drop rates; ok is false when the export has none.
func (e *Export) Rates() (rates []Rate, ok bool, err error) {
	f, ok := e.file("drops", "rates")
	if !ok {
		return nil, false, nil
	}
	err = e.readNDJSON(f, func(l []byte) error {
		var r Rate
		if err := json.Unmarshal(l, &r); err != nil {
			return err
		}
		rates = append(rates, r)
		return nil
	})
	return rates, true, err
}

// Edges streams the export's edges of one relation; ok is false when the export has none.
func (e *Export) Edges(rel string) (edges []Edge, ok bool, err error) {
	f, ok := e.file("edges", "edges")
	if !ok {
		return nil, false, nil
	}
	err = e.readNDJSON(f, func(l []byte) error {
		var r Edge
		if err := json.Unmarshal(l, &r); err != nil {
			return err
		}
		if r.Rel == rel {
			edges = append(edges, r)
		}
		return nil
	})
	return edges, true, err
}

// corpusInto streams the export's corpus records of the kinds into c; ok is false when the
// export has none.
func (e *Export) corpusInto(c *Corpus, kinds []string) (ok bool, err error) {
	f, ok := e.file("corpus", "records")
	if !ok {
		return false, nil
	}
	err = e.readNDJSON(f, func(l []byte) error {
		var head struct {
			Kind string `json:"kind"`
		}
		if err := json.Unmarshal(l, &head); err != nil {
			return err
		}
		for _, k := range kinds {
			if k == head.Kind {
				return c.add(k, json.RawMessage(l))
			}
		}
		return nil
	})
	return true, err
}

// exportAttribution is the attribution of an export's rows.
func (e *Export) attribution() Attribution {
	return Attribution{Licence: License, LicenceURL: "https://creativecommons.org/licenses/by-sa/4.0/", Credit: Credit, Source: "https://everloot.app/database", Terms: "https://everloot.app/terms#licences"}
}

// SourceVersion is the manifest version of a source read through origin: the plain dataAsOf
// when no route was recorded, else the route and the date ("export <path> dataAsOf …",
// "api dataAsOf …").
func SourceVersion(origin, dataAsOf string) string {
	if origin == "" || dataAsOf == "" || dataAsOf == "unavailable" {
		return dataAsOf
	}
	return origin + " dataAsOf " + dataAsOf
}
