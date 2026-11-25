// Package carry finds the newest published dataset release that still holds a table's
// rows, so a build that could not reach Everloot carries those rows forward instead of
// publishing the table empty.
package carry

import (
	"archive/tar"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/WoW-Companion/data/internal/manifest"
	"github.com/klauspost/compress/zstd"
)

const (
	// Carried is the Version of a manifest source whose rows were carried forward.
	Carried = "carried-forward"
	// Unavailable is the Version a build stamps on an Everloot source it could not fetch.
	Unavailable = "unavailable"
	tagPrefix   = "dataset-"
	maxAsset    = 1 << 30
)

type release struct {
	Tag       string
	Published time.Time
}

type loaded struct {
	m     manifest.Manifest
	err   error
	files map[string][]byte // the archive's tables by file name, read on first need
	ferr  error
	read  bool
}

// Finder reads published dataset releases of one repository. API is the GitHub API base,
// Download the public download base (https://github.com); the zero HTTP client is a
// 5-minute one.
type Finder struct {
	API, Download, Repo, Build, Token, UserAgent string
	HTTP                                         *http.Client

	listed   bool
	releases []release
	cache    map[string]*loaded
	Log      func(format string, args ...any)
}

func (f *Finder) logf(format string, args ...any) {
	if f.Log != nil {
		f.Log(format, args...)
	}
}

func (f *Finder) client() *http.Client {
	if f.HTTP == nil {
		f.HTTP = &http.Client{Timeout: 5 * time.Minute}
	}
	return f.HTTP
}

func (f *Finder) get(url string) ([]byte, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	if f.UserAgent != "" {
		req.Header.Set("User-Agent", f.UserAgent)
	}
	resp, err := f.client().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: HTTP %d", url, resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, maxAsset))
}

// list fills f.releases: the dataset releases, this build's own tag first and the rest
// newest first.
func (f *Finder) list() error {
	if f.listed {
		return nil
	}
	req, err := http.NewRequest("GET", f.API+"/repos/"+f.Repo+"/releases?per_page=50", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if f.Token != "" {
		req.Header.Set("Authorization", "Bearer "+f.Token)
	}
	resp, err := f.client().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("list releases of %s: HTTP %d", f.Repo, resp.StatusCode)
	}
	var raw []struct {
		Tag       string    `json:"tag_name"`
		Draft     bool      `json:"draft"`
		Published time.Time `json:"published_at"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(&raw); err != nil {
		return fmt.Errorf("list releases of %s: %w", f.Repo, err)
	}
	for _, r := range raw {
		if !r.Draft && strings.HasPrefix(r.Tag, tagPrefix) {
			f.releases = append(f.releases, release{r.Tag, r.Published})
		}
	}
	own := tagPrefix + f.Build
	sort.SliceStable(f.releases, func(i, j int) bool {
		a, b := f.releases[i], f.releases[j]
		if (a.Tag == own) != (b.Tag == own) {
			return a.Tag == own
		}
		return a.Published.After(b.Published)
	})
	f.listed = true
	return nil
}

func (f *Finder) load(r release) *loaded {
	if f.cache == nil {
		f.cache = map[string]*loaded{}
	}
	if l, ok := f.cache[r.Tag]; ok {
		return l
	}
	l := &loaded{}
	f.cache[r.Tag] = l
	build := strings.TrimPrefix(r.Tag, tagPrefix)
	b, err := f.get(f.assetURL(r.Tag, "manifest-"+build+".json"))
	if err != nil {
		l.err = err
		return l
	}
	if err := manifest.Validate(b); err != nil {
		l.err = fmt.Errorf("manifest of %s: %w", r.Tag, err)
		return l
	}
	l.m, l.err = manifest.Decode(b)
	return l
}

func (f *Finder) assetURL(tag, name string) string {
	return f.Download + "/" + f.Repo + "/releases/download/" + tag + "/" + name
}

func (f *Finder) files(r release, l *loaded) map[string][]byte {
	if l.read {
		return l.files
	}
	l.read = true
	build := strings.TrimPrefix(r.Tag, tagPrefix)
	b, err := f.get(f.assetURL(r.Tag, "wowc-dataset-"+build+".tar.zst"))
	if err != nil {
		l.ferr = err
		return nil
	}
	l.files, l.ferr = Extract(b)
	return l.files
}

// Extract reads the .parquet files of a zstd-compressed dataset tarball, by base name.
func Extract(archive []byte) (map[string][]byte, error) {
	zr, err := zstd.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	out := map[string][]byte{}
	tr := tar.NewReader(zr)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
		if h.Typeflag != tar.TypeReg || !strings.HasSuffix(h.Name, ".parquet") {
			continue
		}
		b, err := io.ReadAll(io.LimitReader(tr, maxAsset))
		if err != nil {
			return nil, err
		}
		out[path.Base(h.Name)] = b
	}
}

// Previous is the manifest of the newest published release (this build's own tag first),
// the shrink guard's baseline. ok is false when there is no release yet.
func (f *Finder) Previous() (m manifest.Manifest, tag string, ok bool, err error) {
	if err := f.list(); err != nil {
		return m, "", false, err
	}
	for _, r := range f.releases {
		l := f.load(r)
		if l.err != nil {
			f.logf("release %s: manifest unreadable, skipped: %v", r.Tag, l.err)
			continue
		}
		return l.m, r.Tag, true, nil
	}
	return m, "", false, nil
}

// Table returns the rows file of table (for example vendor_sources.parquet) from the
// newest release whose manifest shows it with rows and its Everloot source as fetched
// (or carried from a fetch), never from a release that was itself built empty. The file's
// sha256 is verified against that manifest. src is the manifest source to record: the
// original with Version carried-forward and the revision it came from in its name; a
// source already carried forward keeps its original revision and retrieved time.
func (f *Finder) Table(table string) (data []byte, src manifest.Source, ok bool) {
	if err := f.list(); err != nil {
		f.logf("carry forward %s: %v", table, err)
		return nil, src, false
	}
	for _, r := range f.releases {
		l := f.load(r)
		if l.err != nil {
			continue
		}
		i := slices.IndexFunc(l.m.Tables, func(t manifest.Table) bool { return t.Path == table })
		if i < 0 || l.m.Tables[i].Rows == 0 {
			continue
		}
		t := l.m.Tables[i]
		j := slices.IndexFunc(t.Sources, func(s manifest.Source) bool { return strings.HasPrefix(s.Name, "Everloot") })
		if j < 0 || t.Sources[j].Version == Unavailable {
			continue
		}
		files := f.files(r, l)
		b, have := files[table]
		if !have {
			f.logf("release %s: %s missing from the archive (%v), skipped", r.Tag, table, l.ferr)
			continue
		}
		sum := sha256.Sum256(b)
		if got := hex.EncodeToString(sum[:]); got != t.SHA256 {
			f.logf("release %s: %s sha256 %s does not match its manifest %s, skipped", r.Tag, table, got, t.SHA256)
			continue
		}
		src = t.Sources[j]
		if src.Version != Carried {
			rev := l.m.Revision
			if rev == "" {
				rev = "unknown"
			}
			src.Name += " (carried forward from revision " + rev + " of " + r.Tag + ")"
			src.Version = Carried
		}
		return b, src, true
	}
	return nil, src, false
}
