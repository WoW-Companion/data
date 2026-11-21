package httpget

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestGet(t *testing.T) {
	cases := []struct {
		name       string
		statuses   []int // answers in order; the last repeats
		retryAfter string
		budget     time.Duration
		want       string // body of the 200, or "" for an error
		wantErr    string
		waits      []time.Duration
	}{
		{"ok first time", []int{200}, "", time.Hour, "ok", "", nil},
		{"429 then ok waits Retry-After", []int{429, 200}, "90", time.Hour, "ok", "", []time.Duration{90 * time.Second}},
		{"500 then ok backs off", []int{500, 502, 200}, "", time.Hour, "ok", "", []time.Duration{time.Second, 2 * time.Second}},
		{"404 is final", []int{404, 200}, "", time.Hour, "", "HTTP 404", nil},
		{"attempts run out", []int{429, 429, 429, 429}, "1", time.Hour, "", "HTTP 429", []time.Duration{time.Second, time.Second}},
		{"wait over the budget", []int{429, 200}, "1400", 10 * time.Minute, "", "over the retry budget", nil},
		{"wait over the single cap", []int{429, 200}, "4000", 10 * time.Hour, "", "over the retry budget", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			i := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				s := c.statuses[min(i, len(c.statuses)-1)]
				i++
				if r.Header.Get("User-Agent") != "ua" {
					t.Errorf("user agent %q", r.Header.Get("User-Agent"))
				}
				if s == 429 && c.retryAfter != "" {
					w.Header().Set("Retry-After", c.retryAfter)
				}
				w.WriteHeader(s)
				if s == 200 {
					w.Write([]byte("ok"))
				}
			}))
			defer srv.Close()
			var waits []time.Duration
			g := &Getter{Client: srv.Client(), UserAgent: "ua", Attempts: 3, Base: time.Second, MaxWait: time.Hour, Budget: c.budget,
				Sleep: func(d time.Duration) { waits = append(waits, d) }}
			if c.name == "wait over the single cap" {
				g.MaxWait = 30 * time.Minute
			}
			got, err := g.Get(srv.URL)
			if c.wantErr == "" && (err != nil || string(got) != c.want) {
				t.Fatalf("got %q, %v; want %q", got, err, c.want)
			}
			if c.wantErr != "" && (err == nil || !strings.Contains(err.Error(), c.wantErr)) {
				t.Fatalf("err %v; want it to contain %q", err, c.wantErr)
			}
			if len(waits) != len(c.waits) {
				t.Fatalf("waits %v; want %v", waits, c.waits)
			}
			for j := range waits {
				if waits[j] != c.waits[j] {
					t.Fatalf("waits %v; want %v", waits, c.waits)
				}
			}
		})
	}
}
