// Package release answers the dataset workflow's two questions: what is the latest
// WoW: Forever build on wago.tools, and has a release for it been published yet.
package release

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"time"
)

// TagPrefix starts every dataset release tag: dataset-<build>.
const TagPrefix = "dataset-"

var buildPattern = regexp.MustCompile(`^\d+(\.\d+){3}$`)

var client = &http.Client{Timeout: 30 * time.Second}

// Tag is the release tag for a build.
func Tag(build string) string { return TagPrefix + build }

// LatestBuild asks wago.tools (base) for the newest build of product.
func LatestBuild(base, product, userAgent string) (string, error) {
	req, err := http.NewRequest("GET", base+"/api/builds/latest", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", userAgent)
	body, status, err := do(req)
	if err != nil {
		return "", err
	}
	if status != http.StatusOK {
		return "", fmt.Errorf("wago.tools builds/latest: HTTP %d", status)
	}
	return parseLatest(body, product)
}

func parseLatest(body []byte, product string) (string, error) {
	var m map[string]struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(body, &m); err != nil {
		return "", fmt.Errorf("wago.tools builds/latest: %w", err)
	}
	b, ok := m[product]
	if !ok {
		return "", fmt.Errorf("wago.tools builds/latest: no product %q", product)
	}
	if !buildPattern.MatchString(b.Version) {
		return "", fmt.Errorf("wago.tools builds/latest: product %q has unusable version %q", product, b.Version)
	}
	return b.Version, nil
}

// Exists reports whether repo ("owner/name") has a release with the tag, through
// the GitHub API at api, authenticated with token (may be empty for public repos).
func Exists(api, repo, tag, token string) (bool, error) {
	req, err := http.NewRequest("GET", api+"/repos/"+repo+"/releases/tags/"+url.PathEscape(tag), nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	_, status, err := do(req)
	if err != nil {
		return false, err
	}
	switch status {
	case http.StatusOK:
		return true, nil
	case http.StatusNotFound:
		return false, nil
	}
	return false, fmt.Errorf("github release lookup for %s: HTTP %d", tag, status)
}

func do(req *http.Request) ([]byte, int, error) {
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return b, resp.StatusCode, err
}
