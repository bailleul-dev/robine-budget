package web

import (
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"bailleul.dev/budget-server/internal/core"
)

const (
	// maxJSONBodyBytes caps every non-multipart request body. Real payloads
	// (a transaction, a category, a login) are a few KB.
	maxJSONBodyBytes = 1 << 20
	// maxUploadBodyBytes caps multipart bodies (CSV imports). The handlers
	// keep at most 32 MiB in memory and spill the rest to disk, so the whole
	// body needs its own ceiling.
	maxUploadBodyBytes = 40 << 20
)

// clientIP returns the address to rate-limit a request on. Behind a reverse
// proxy the socket peer is the proxy, so the real client is read from
// X-Forwarded-For — but only as many entries from the right as there are
// trusted proxies (settings.TrustedProxyHops): everything further left is
// client-supplied and spoofable. With no trusted proxy configured the header
// is ignored altogether.
func clientIP(r *http.Request, trustedProxyHops int) string {
	if trustedProxyHops > 0 {
		var hops []string
		for _, v := range r.Header.Values("X-Forwarded-For") {
			for _, part := range strings.Split(v, ",") {
				if p := strings.TrimSpace(part); p != "" {
					hops = append(hops, p)
				}
			}
		}
		if len(hops) > 0 {
			idx := len(hops) - trustedProxyHops
			if idx < 0 {
				idx = 0
			}
			return hops[idx]
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// SecurityHeadersMiddleware sets the response headers that cost nothing and
// close whole classes of browser attacks. The OAuth pages and the
// verification/reset pages set their own stricter Content-Security-Policy.
func SecurityHeadersMiddleware(settings core.Settings) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("X-Frame-Options", "DENY")
			h.Set("Referrer-Policy", "no-referrer")
			h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
			if settings.CookieSecure {
				h.Set("Strict-Transport-Security", "max-age=15552000")
			}
			path := r.URL.Path
			if strings.HasPrefix(path, "/api/") && !strings.HasPrefix(path, "/api/updates/") {
				// Personal financial data: never cached by a browser or an
				// intermediary, and an API response is never a document.
				h.Set("Cache-Control", "no-store")
				h.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
			}
			next.ServeHTTP(w, r)
		})
	}
}

// LimitBodiesMiddleware bounds how much of a request body any handler can be
// made to read.
func LimitBodiesMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Body != nil && r.Body != http.NoBody {
			limit := int64(maxJSONBodyBytes)
			if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/") {
				limit = maxUploadBodyBytes
			}
			r.Body = http.MaxBytesReader(w, r.Body, limit)
		}
		next.ServeHTTP(w, r)
	})
}

// rateLimiter is a per-key token bucket held in memory: `burst` requests are
// allowed at once, then one more every `every`. In-memory is enough while the
// server runs as a single replica (see the k8s manifest); with several
// replicas each would enforce its own share, which only loosens the limit.
type rateLimiter struct {
	mu      sync.Mutex
	every   time.Duration
	burst   float64
	buckets map[string]*bucket
	now     func() time.Time
	// sweepAt is when idle buckets were last dropped, to bound memory.
	sweepAt time.Time
}

type bucket struct {
	tokens float64
	last   time.Time
}

func newRateLimiter(every time.Duration, burst int) *rateLimiter {
	return &rateLimiter{every: every, burst: float64(burst), buckets: map[string]*bucket{}, now: time.Now}
}

// take consumes one token for key. It reports whether the request may go
// ahead and, when it may not, how long until a token is available.
func (l *rateLimiter) take(key string) (ok bool, retryAfter time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	l.sweep(now)
	b := l.buckets[key]
	if b == nil {
		b = &bucket{tokens: l.burst, last: now}
		l.buckets[key] = b
	}
	b.tokens += float64(now.Sub(b.last)) / float64(l.every)
	if b.tokens > l.burst {
		b.tokens = l.burst
	}
	b.last = now
	if b.tokens < 1 {
		return false, time.Duration((1 - b.tokens) * float64(l.every))
	}
	b.tokens--
	return true, 0
}

// refund gives back a token, used when an attempt turned out to be legitimate
// (a successful login shouldn't count against the failed-attempt budget).
func (l *rateLimiter) refund(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if b := l.buckets[key]; b != nil && b.tokens < l.burst {
		b.tokens++
	}
}

// sweep drops buckets that have been idle long enough to be full again.
// Called with l.mu held.
func (l *rateLimiter) sweep(now time.Time) {
	if now.Sub(l.sweepAt) < time.Minute {
		return
	}
	l.sweepAt = now
	idle := time.Duration(l.burst) * l.every
	for k, b := range l.buckets {
		if now.Sub(b.last) > idle {
			delete(l.buckets, k)
		}
	}
}

// rateLimited wraps a handler so each client IP gets the limiter's budget.
func rateLimited(l *rateLimiter, trustedProxyHops int, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ip := clientIP(r, trustedProxyHops)
		if ok, retry := l.take(ip); !ok {
			// Logged so a misconfigured proxy shows up at once: if every
			// blocked request carries the same address (a node or pod IP), the
			// real client IP isn't reaching the server and the limit is
			// being shared by everybody — see TRUSTED_PROXY_HOPS.
			log.Printf("rate limit: %s %s from %s refused (retry in %s)", r.Method, r.URL.Path, ip, retry.Round(time.Second))
			writeTooManyRequests(w, retry)
			return
		}
		next(w, r)
	}
}

func writeTooManyRequests(w http.ResponseWriter, retryAfter time.Duration) {
	secs := int(retryAfter.Seconds()) + 1
	w.Header().Set("Retry-After", strconv.Itoa(secs))
	http.Error(w, `{"detail":"too many requests, slow down"}`, http.StatusTooManyRequests)
}
