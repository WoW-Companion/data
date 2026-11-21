package release

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLatestBuild(t *testing.T) {
	cases := []struct {
		name    string
		status  int
		body    string
		want    string
		wantErr bool
	}{
		{"ok", 200, `{"wow":{"version":"12.0.0.1"},"wowf":{"version":"1.60.1.71037"}}`, "1.60.1.71037", false},
		{"missing product", 200, `{"wow":{"version":"12.0.0.1"}}`, "", true},
		{"odd version", 200, `{"wowf":{"version":"latest; rm"}}`, "", true},
		{"not json", 200, `<html>`, "", true},
		{"server error", 503, ``, "", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/builds/latest" || r.Header.Get("User-Agent") != "ua" {
					t.Errorf("unexpected request %s ua=%q", r.URL.Path, r.Header.Get("User-Agent"))
				}
				w.WriteHeader(c.status)
				w.Write([]byte(c.body))
			}))
			defer srv.Close()
			got, err := LatestBuild(srv.URL, "wowf", "ua")
			if (err != nil) != c.wantErr || got != c.want {
				t.Fatalf("got %q, %v; want %q, err=%v", got, err, c.want, c.wantErr)
			}
		})
	}
}

func TestExists(t *testing.T) {
	cases := []struct {
		name    string
		status  int
		want    bool
		wantErr bool
	}{
		{"published", 200, true, false},
		{"absent", 404, false, false},
		{"forbidden", 403, false, true},
		{"server error", 500, false, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/repos/o/r/releases/tags/dataset-1.2.3.4" || r.Header.Get("Authorization") != "Bearer tok" {
					t.Errorf("unexpected request %s auth=%q", r.URL.Path, r.Header.Get("Authorization"))
				}
				w.WriteHeader(c.status)
			}))
			defer srv.Close()
			got, err := Exists(srv.URL, "o/r", Tag("1.2.3.4"), "tok")
			if (err != nil) != c.wantErr || got != c.want {
				t.Fatalf("got %v, %v; want %v, err=%v", got, err, c.want, c.wantErr)
			}
		})
	}
}

func TestNotes(t *testing.T) {
	cases := []struct {
		name, prev, changes string
		has, lacks          []string
	}{
		{"first release", "", "", []string{"First release: there is no earlier dataset", "game build 1.2.3.4", "CC BY-SA 4.0", "Everloot", "wago.tools"}, []string{"Changes since"}},
		{"with a previous release", "dataset-1.2.3.3", "## Tables\n\n- items\n\n", []string{"Changes since dataset-1.2.3.3.\n\n## Tables\n\n- items\n\nWoW: Forever", "Verify:"}, []string{"First release"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Notes("1.2.3.4", c.prev, c.changes)
			for _, s := range c.has {
				if !strings.Contains(got, s) {
					t.Errorf("notes lack %q:\n%s", s, got)
				}
			}
			for _, s := range c.lacks {
				if strings.Contains(got, s) {
					t.Errorf("notes hold %q:\n%s", s, got)
				}
			}
		})
	}
}
