package release

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func tree(t *testing.T) string {
	root := t.TempDir()
	for p, c := range map[string]string{
		"curated/talents/a.json": "{}", "internal/x/x.go": "package x", "internal/x/x_test.go": "package x",
		"cmd/m/main.go": "package main", "schemas/s.json": "{}",
	} {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(root, p)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, p), []byte(c), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestRevision(t *testing.T) {
	root := tree(t)
	a, err := Revision(root)
	if err != nil || len(a) != 12 {
		t.Fatalf("got %q, %v", a, err)
	}
	if b, _ := Revision(root); b != a {
		t.Errorf("unchanged tree: %s != %s", b, a)
	}
	os.WriteFile(filepath.Join(root, "internal/x/x_test.go"), []byte("changed"), 0o644)
	if b, _ := Revision(root); b != a {
		t.Errorf("a test file changed the revision")
	}
	os.WriteFile(filepath.Join(root, "curated/talents/a.json"), []byte(`{"a":1}`), 0o644)
	if b, _ := Revision(root); b == a {
		t.Errorf("a curated change kept revision %s", a)
	}
}

func TestPublishedRevisionAndNeedsPublish(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/repos/o/r/releases/tags/"):
			w.Write([]byte(`{"assets":[{"name":"manifest-1.2.3.4.json","url":"` + srv.URL + `/asset"}]}`))
		case r.URL.Path == "/asset":
			w.Write([]byte(`{"revision":"abc123abc123"}`))
		}
	}))
	defer srv.Close()
	got, err := PublishedRevision(srv.URL, "o", "dataset-1.2.3.4", "1.2.3.4", "")
	if err == nil {
		t.Fatalf("bad repo should fail, got %q", got)
	}
	got, err = PublishedRevision(srv.URL, "o/r", "dataset-1.2.3.4", "1.2.3.4", "")
	if err != nil || got != "abc123abc123" {
		t.Fatalf("got %q, %v", got, err)
	}
	for _, c := range []struct {
		exists   bool
		pub, cur string
		want     bool
	}{{false, "", "x", true}, {true, "x", "x", false}, {true, "y", "x", true}, {true, "", "x", true}} {
		if NeedsPublish(c.exists, c.pub, c.cur) != c.want {
			t.Errorf("%+v", c)
		}
	}
}

func TestNotesRevisionLine(t *testing.T) {
	if got := NotesAt("1.2.3.4", "", "", "abc123abc123", "2026-11-23"); !strings.Contains(got, "Revision: abc123abc123 (2026-11-23).") {
		t.Errorf("no revision line:\n%s", got)
	}
	if strings.Contains(Notes("1.2.3.4", "", ""), "Revision:") {
		t.Error("revision line without a revision")
	}
}
