// Package httpget is a GET that waits out rate limits and brief server errors, for the
// fetches the dataset workflow makes from a shared runner address.
package httpget

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Getter retries a GET on a network error, HTTP 429 and HTTP 5xx. A 429's Retry-After
// is honoured; other waits double from Base. Every wait counts against Budget, shared
// by all requests of one Getter, and a wait that would exceed it fails the request.
type Getter struct {
	Client    *http.Client
	UserAgent string        // sent when not empty
	BearerKey string        // sent as "Authorization: Bearer <key>" when not empty; never logged
	Attempts  int           // tries per request, at least 1
	Base      time.Duration // first backoff wait
	MaxWait   time.Duration // longest single wait
	Budget    time.Duration // longest total wait
	Sleep     func(time.Duration)

	waited time.Duration
}

// RetryAfter reads a Retry-After header in seconds; 0 when absent or not a number.
func RetryAfter(h string) time.Duration {
	n, err := strconv.Atoi(h)
	if err != nil || n < 0 {
		return 0
	}
	return time.Duration(n) * time.Second
}

// retryable reports whether a status is worth another try.
func retryable(status int) bool {
	return status == http.StatusTooManyRequests || status >= 500
}

// wait is how long to pause before try number n (1-based) after a failure.
func (g *Getter) wait(n int, retryAfter string) time.Duration {
	d := g.Base << (n - 1)
	if ra := RetryAfter(retryAfter); ra > 0 {
		d = ra
	}
	return d
}

// redact removes the key from an error's text, wherever a URL or a transport error put it.
func (g *Getter) redact(err error) error {
	if err == nil || g.BearerKey == "" || !strings.Contains(err.Error(), g.BearerKey) {
		return err
	}
	return fmt.Errorf("%s", strings.ReplaceAll(err.Error(), g.BearerKey, "[redacted]"))
}

// Get returns the body of a 200 answer from u. Errors never carry the bearer key.
func (g *Getter) Get(u string) ([]byte, error) {
	b, err := g.get(u)
	return b, g.redact(err)
}

func (g *Getter) get(u string) ([]byte, error) {
	var last error
	for n := 1; ; n++ {
		body, status, retryAfter, err := g.once(u)
		if err == nil && status == http.StatusOK {
			return body, nil
		}
		if err == nil {
			last = fmt.Errorf("%s: HTTP %d (Retry-After %q)", u, status, retryAfter)
			if !retryable(status) {
				return nil, last
			}
		} else {
			last = fmt.Errorf("%s: %w", u, err)
		}
		if n >= g.Attempts {
			return nil, last
		}
		d := g.wait(n, retryAfter)
		if d > g.MaxWait || g.waited+d > g.Budget {
			return nil, fmt.Errorf("%w; the %s wait it asks for is over the retry budget", last, d)
		}
		g.waited += d
		g.Sleep(d)
	}
}

func (g *Getter) once(u string) (body []byte, status int, retryAfter string, err error) {
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, 0, "", err
	}
	if g.UserAgent != "" {
		req.Header.Set("User-Agent", g.UserAgent)
	}
	if g.BearerKey != "" {
		req.Header.Set("Authorization", "Bearer "+g.BearerKey)
	}
	resp, err := g.Client.Do(req)
	if err != nil {
		return nil, 0, "", err
	}
	defer resp.Body.Close()
	body, err = io.ReadAll(resp.Body)
	return body, resp.StatusCode, resp.Header.Get("Retry-After"), err
}
