package release

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// RevisionDirs are the trees whose content decides a dataset's revision: the curated
// files, the table-building code and the schema registry.
var RevisionDirs = []string{"curated", "internal", "cmd", "schemas"}

// Revision is a short hash of the inputs we control under root: every file in
// RevisionDirs (Go test files excluded, they change no output), by path and content.
// It is stable for an unchanged tree.
func Revision(root string) (string, error) {
	var files []string
	for _, d := range RevisionDirs {
		err := filepath.WalkDir(filepath.Join(root, d), func(p string, e fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if e.IsDir() || strings.HasSuffix(p, "_test.go") {
				return nil
			}
			rel, err := filepath.Rel(root, p)
			if err != nil {
				return err
			}
			files = append(files, filepath.ToSlash(rel))
			return nil
		})
		if err != nil {
			return "", err
		}
	}
	sort.Strings(files)
	h := sha256.New()
	for _, f := range files {
		b, err := os.ReadFile(filepath.Join(root, f))
		if err != nil {
			return "", err
		}
		fmt.Fprintf(h, "%s\x00%d\x00", f, len(b))
		h.Write(b)
	}
	return hex.EncodeToString(h.Sum(nil))[:12], nil
}

// PublishedRevision reads the revision from the manifest asset of release tag in repo.
// A release whose manifest has no revision (published before revisions) reports "".
func PublishedRevision(api, repo, tag, build, token string) (string, error) {
	req, err := http.NewRequest("GET", api+"/repos/"+repo+"/releases/tags/"+tag, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	body, status, err := do(req)
	if err != nil {
		return "", err
	}
	if status != http.StatusOK {
		return "", fmt.Errorf("github release lookup for %s: HTTP %d", tag, status)
	}
	var rel struct {
		Assets []struct {
			Name string `json:"name"`
			URL  string `json:"url"`
		} `json:"assets"`
	}
	if err := json.Unmarshal(body, &rel); err != nil {
		return "", err
	}
	want := "manifest-" + build + ".json"
	for _, a := range rel.Assets {
		if a.Name != want {
			continue
		}
		r, err := http.NewRequest("GET", a.URL, nil)
		if err != nil {
			return "", err
		}
		r.Header.Set("Accept", "application/octet-stream")
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		mb, st, err := do(r)
		if err != nil {
			return "", err
		}
		if st != http.StatusOK {
			return "", fmt.Errorf("download %s: HTTP %d", want, st)
		}
		var m struct {
			Revision string `json:"revision"`
		}
		if err := json.Unmarshal(mb, &m); err != nil {
			return "", fmt.Errorf("%s: %w", want, err)
		}
		return m.Revision, nil
	}
	return "", nil
}

// NeedsPublish reports whether the workflow must build and publish: there is no
// release for the build, or its manifest carries a different revision from the tree's.
func NeedsPublish(exists bool, published, current string) bool {
	return !exists || published != current
}
