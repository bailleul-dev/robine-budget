package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"bailleul.dev/budget-server/internal/core"
)

func TestClientIP(t *testing.T) {
	req := func(remote string, xff ...string) *http.Request {
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = remote
		for _, v := range xff {
			r.Header.Add("X-Forwarded-For", v)
		}
		return r
	}
	cases := []struct {
		name string
		r    *http.Request
		hops int
		want string
	}{
		{"no proxy ignores a spoofed header", req("10.0.0.9:1234", "6.6.6.6"), 0, "10.0.0.9"},
		{"one proxy takes the last entry", req("10.0.0.9:1", "6.6.6.6, 203.0.113.7"), 1, "203.0.113.7"},
		{"two proxies take the second from the right", req("10.0.0.9:1", "6.6.6.6, 203.0.113.7, 172.16.0.2"), 2, "203.0.113.7"},
		{"fewer entries than hops falls back to the leftmost", req("10.0.0.9:1", "203.0.113.7"), 3, "203.0.113.7"},
		{"trusted proxy but no header uses the socket", req("10.0.0.9:1"), 1, "10.0.0.9"},
	}
	for _, c := range cases {
		if got := clientIP(c.r, c.hops); got != c.want {
			t.Errorf("%s: clientIP = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestRateLimiter_BurstThenRefill(t *testing.T) {
	now := time.Unix(1000, 0)
	l := newRateLimiter(10*time.Second, 3)
	l.now = func() time.Time { return now }

	for i := 0; i < 3; i++ {
		if ok, _ := l.take("a"); !ok {
			t.Fatalf("request %d refused inside the burst", i+1)
		}
	}
	ok, retry := l.take("a")
	if ok || retry <= 0 || retry > 10*time.Second {
		t.Fatalf("4th request: ok=%v retry=%v, want refused with a retry delay <= 10s", ok, retry)
	}
	if ok, _ := l.take("b"); !ok {
		t.Error("another key must have its own budget")
	}
	now = now.Add(10 * time.Second)
	if ok, _ := l.take("a"); !ok {
		t.Error("one token should have been refilled after 10s")
	}
	if ok, _ := l.take("a"); ok {
		t.Error("only one token should have been refilled")
	}
}

func TestRateLimiter_Refund(t *testing.T) {
	l := newRateLimiter(time.Hour, 1)
	if ok, _ := l.take("k"); !ok {
		t.Fatal("first take refused")
	}
	l.refund("k")
	if ok, _ := l.take("k"); !ok {
		t.Error("a refunded token should be usable again")
	}
}

func TestRateLimiter_SweepDropsIdleBuckets(t *testing.T) {
	now := time.Unix(1000, 0)
	l := newRateLimiter(time.Second, 2)
	l.now = func() time.Time { return now }
	l.take("old")
	now = now.Add(5 * time.Minute)
	l.take("new")
	if _, ok := l.buckets["old"]; ok {
		t.Error("an idle bucket should have been swept")
	}
}

func TestRateLimited_Returns429WithRetryAfter(t *testing.T) {
	l := newRateLimiter(time.Hour, 1)
	h := rateLimited(l, 0, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
	do := func() *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/", nil)
		r.RemoteAddr = "192.0.2.1:5000"
		h(rec, r)
		return rec
	}
	if got := do().Code; got != http.StatusNoContent {
		t.Fatalf("first request = %d, want 204", got)
	}
	rec := do()
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") == "" {
		t.Fatalf("second request = %d (Retry-After %q), want 429 with Retry-After", rec.Code, rec.Header().Get("Retry-After"))
	}
}

func TestSecurityHeaders(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	h := SecurityHeadersMiddleware(core.Settings{CookieSecure: true})(next)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/accounts", nil))
	for header, want := range map[string]string{
		"X-Content-Type-Options":    "nosniff",
		"X-Frame-Options":           "DENY",
		"Referrer-Policy":           "no-referrer",
		"Cache-Control":             "no-store",
		"Strict-Transport-Security": "max-age=15552000",
	} {
		if got := rec.Header().Get(header); got != want {
			t.Errorf("%s = %q, want %q", header, got, want)
		}
	}
	if !strings.Contains(rec.Header().Get("Content-Security-Policy"), "default-src 'none'") {
		t.Error("API responses need a restrictive CSP")
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/updates/manifest", nil))
	if rec.Header().Get("Cache-Control") != "" {
		t.Error("OTA update assets must stay cacheable")
	}

	insecure := SecurityHeadersMiddleware(core.Settings{})(next)
	rec = httptest.NewRecorder()
	insecure.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Header().Get("Strict-Transport-Security") != "" {
		t.Error("HSTS must not be sent when cookies aren't secure (plain-HTTP dev)")
	}
}

func TestLimitBodies(t *testing.T) {
	var readErr error
	h := LimitBodiesMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 2<<20)
		_, readErr = r.Body.Read(buf)
		for readErr == nil {
			_, readErr = r.Body.Read(buf)
		}
	}))
	r := httptest.NewRequest("POST", "/", strings.NewReader(strings.Repeat("x", 2<<20)))
	r.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(httptest.NewRecorder(), r)
	if readErr == nil || !strings.Contains(readErr.Error(), "too large") {
		t.Errorf("read error = %v, want a request-too-large error for a 2 MiB JSON body", readErr)
	}
}
