package everloot

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// fake records each request's Authorization header, limit and path.
type fake struct {
	mu    sync.Mutex
	auth  []string
	limit []string
	paths []string
	fail  bool
}

func (f *fake) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.auth = append(f.auth, r.Header.Get("Authorization"))
	f.limit = append(f.limit, r.URL.Query().Get("limit"))
	f.paths = append(f.paths, r.URL.Path+"?kinds="+r.URL.Query().Get("kinds"))
	f.mu.Unlock()
	if f.fail {
		http.Error(w, "bad", http.StatusBadRequest)
		return
	}
	switch r.URL.Path {
	case DropsPath:
		w.Write([]byte(`{"rates":[],"next":null}`))
	case EdgesPath:
		w.Write([]byte(`{"edges":[],"next":null}`))
	case CorpusPath:
		w.Write([]byte(`{"records":[],"next":null}`))
	default:
		w.Write([]byte(`{}`))
	}
}

func TestKeyHeaderLimitAndKinds(t *testing.T) {
	Delay = time.Millisecond
	for _, tc := range []struct {
		key, limit string
		loot       bool
	}{{"", "500", false}, {"sekret-key-123", "5000", true}} {
		t.Setenv(KeyEnv, tc.key)
		f := &fake{}
		srv := httptest.NewServer(f)
		dir := t.TempDir()
		if _, err := Fetch(srv.URL, dir, "now"); err != nil {
			t.Fatal(err)
		}
		if _, err := FetchCorpus(srv.URL, dir, "now", "1.0.0"); err != nil {
			t.Fatal(err)
		}
		srv.Close()
		for i, a := range f.auth {
			want := ""
			if tc.key != "" {
				want = "Bearer " + tc.key
			}
			if a != want {
				t.Fatalf("request %s: Authorization %q, want %q", f.paths[i], a, want)
			}
		}
		for i, l := range f.limit {
			if strings.Contains(f.paths[i], "settled") || l == "" {
				continue
			}
			if l != tc.limit {
				t.Errorf("request %s: limit %s, want %s", f.paths[i], l, tc.limit)
			}
		}
		all := strings.Join(f.paths, " ")
		if got := strings.Contains(all, "kinds=loot"); got != tc.loot {
			t.Errorf("key %q: loot fetched %v, want %v", tc.key, got, tc.loot)
		}
		if !strings.Contains(all, EdgesPath) {
			t.Errorf("edges not fetched: %s", all)
		}
	}
}

func TestKeyNeverInErrors(t *testing.T) {
	Delay = time.Millisecond
	const key = "sekret-key-123"
	t.Setenv(KeyEnv, key)
	srv := httptest.NewServer(&fake{fail: true})
	// The key in a URL (a hostile "next") and a refused connection both must not echo it.
	_, err := Fetch(srv.URL+"/x?token="+key, t.TempDir(), "now")
	srv.Close()
	if err == nil || strings.Contains(err.Error(), key) {
		t.Fatalf("error %v", err)
	}
	g := newGetter()
	g.Sleep = func(time.Duration) {}
	_, err = g.Get("http://127.0.0.1:1/?k=" + key)
	if err == nil || strings.Contains(err.Error(), key) {
		t.Fatalf("transport error %v", err)
	}
}
