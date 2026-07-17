package middleware

import (
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/httprate"
	httprateredis "github.com/go-chi/httprate-redis"
	"github.com/redis/go-redis/v9"
)

// rateLimitedBody is the 429 payload. It matches the app's {error,code} error
// envelope (see internal/handler/response.go) so the frontend renders it and —
// crucially — does NOT trip its 401→global-logout path (that only fires on 401).
const rateLimitedBody = `{"error":"Too many requests. Please slow down.","code":"rate_limited"}`

// KeyByTrustedIP resolves the real client IP behind Railway's proxy in a
// spoof-resistant way, then buckets IPv6 clients by /64 (httprate.CanonicalizeIP).
//
// httprate's own KeyByRealIP is deprecated because trusting client-supplied IP
// headers is only safe when a trusted proxy sanitizes them first. Railway's edge
// does exactly that: it OVERWRITES inbound X-Real-IP and X-Forwarded-For with the
// true client IP (verified against the live edge — a forged X-Real-IP / left-most
// XFF is stripped), and sets X-Real-IP to that address. Trust order:
//  1. X-Real-IP — Railway's edge sets this to the true client IP; not forgeable.
//  2. LEFT-most X-Forwarded-For entry — the client the first trusted proxy saw.
//     (The RIGHT-most entry is Railway's own edge proxy IP and rotates per
//     request, so it must NOT be used as the key.)
//  3. r.RemoteAddr — the TCP peer (the proxy in prod, the client in local dev).
//
// Railway does NOT populate X-Envoy-External-Address, so it is intentionally not
// used. The per-user limiter protects authenticated routes regardless of IP trust,
// so IP-keying only has to be right for the auth and global tiers.
func KeyByTrustedIP(r *http.Request) (string, error) {
	if xrip := strings.TrimSpace(r.Header.Get("X-Real-IP")); xrip != "" {
		return httprate.CanonicalizeIP(hostOnly(xrip)), nil
	}
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if first := strings.TrimSpace(strings.Split(xff, ",")[0]); first != "" {
			return httprate.CanonicalizeIP(hostOnly(first)), nil
		}
	}
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		ip = r.RemoteAddr
	}
	return httprate.CanonicalizeIP(ip), nil
}

// hostOnly strips a :port suffix when present (X-Forwarded-For / Envoy entries
// occasionally carry one). Returns the input unchanged when there's no port.
func hostOnly(s string) string {
	if host, _, err := net.SplitHostPort(s); err == nil {
		return host
	}
	return s
}

// KeyByUserID keys on the authenticated user injected by Authenticate. It only
// runs inside the protected group, where a userID is always present; it falls
// back to the trusted IP defensively if the userID is somehow empty.
func KeyByUserID(r *http.Request) (string, error) {
	if uid := UserIDFromContext(r.Context()); uid != "" {
		return "user:" + uid, nil
	}
	return KeyByTrustedIP(r)
}

// rateLimitedHandler writes the 429 response. httprate has already set
// Retry-After and the X-RateLimit-* headers by the time this runs.
func rateLimitedHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusTooManyRequests)
	_, _ = w.Write([]byte(rateLimitedBody))
}

// rateLimiterErrorHandler is a near-unreachable backstop: the key funcs never
// return an error, and Redis errors are absorbed by httprate-redis's local
// fallback (see NewRateLimiters). If it ever fires, log and return 503 rather
// than httprate's default 428 Precondition Required.
func rateLimiterErrorHandler(w http.ResponseWriter, r *http.Request, err error) {
	log.Printf("rate limiter error: %v", err)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusServiceUnavailable)
	_, _ = w.Write([]byte(`{"error":"rate limiter unavailable","code":"rate_limiter_error"}`))
}

// RateLimitOptions is the subset of runtime config the limiters need. Kept local
// to this package so middleware doesn't depend on the config package; main.go
// maps config.Config onto it.
type RateLimitOptions struct {
	Enabled        bool
	RedisURL       string
	GlobalRequests int
	GlobalWindow   time.Duration
	UserRequests   int
	UserWindow     time.Duration
	AuthRequests   int
	AuthWindow     time.Duration
}

// RateLimiters bundles the three limiter middlewares. Enabled is the master
// switch; when false, all three are no-op passthroughs (and no Redis client is
// opened). Close releases the shared Redis client, if any.
type RateLimiters struct {
	Enabled bool
	// Global limits per client IP across the whole /api surface.
	Global func(http.Handler) http.Handler
	// User limits per authenticated user; use only inside an Authenticate group.
	User func(http.Handler) http.Handler
	// Auth is a tighter per-IP limit for the public auth routes (brute force/spam).
	Auth func(http.Handler) http.Handler

	redis *redis.Client
}

func passthrough(next http.Handler) http.Handler { return next }

// NewRateLimiters builds the limiter set from opts. When opts.Enabled is false the
// three middlewares are passthroughs. When opts.RedisURL is set, counters live in
// Redis via one shared client (per-tier key prefix) with httprate-redis's local
// in-memory fallback if Redis is unreachable — so a Redis outage degrades to
// per-instance limiting rather than a hard block. When RedisURL is empty, each
// limiter uses httprate's default in-process in-memory counter (local dev / CI).
func NewRateLimiters(opts RateLimitOptions) *RateLimiters {
	rl := &RateLimiters{Enabled: opts.Enabled}
	if !opts.Enabled {
		rl.Global, rl.User, rl.Auth = passthrough, passthrough, passthrough
		return rl
	}

	if opts.RedisURL != "" {
		if ro, err := redis.ParseURL(opts.RedisURL); err != nil {
			log.Printf("rate limiter: invalid REDIS_URL, using in-memory counters: %v", err)
		} else {
			// Short timeouts + no retries so the in-memory fallback activates fast
			// when Redis is unreachable (required when bringing your own client;
			// httprate-redis only auto-tunes the client it creates itself).
			ro.DialTimeout = 3 * time.Second
			ro.ReadTimeout = 500 * time.Millisecond
			ro.WriteTimeout = 500 * time.Millisecond
			ro.MaxRetries = -1
			rl.redis = redis.NewClient(ro)
		}
	}

	rl.Global = buildLimiter("global", opts.GlobalRequests, opts.GlobalWindow, KeyByTrustedIP, rl.redis)
	rl.User = buildLimiter("user", opts.UserRequests, opts.UserWindow, KeyByUserID, rl.redis)
	rl.Auth = buildLimiter("auth", opts.AuthRequests, opts.AuthWindow, KeyByTrustedIP, rl.redis)
	return rl
}

// buildLimiter constructs one httprate limiter. When rdb is non-nil the counter
// is Redis-backed (shared client, tier-scoped key prefix) with a local fallback.
func buildLimiter(tier string, requests int, window time.Duration, key httprate.KeyFunc, rdb *redis.Client) func(http.Handler) http.Handler {
	options := []httprate.Option{
		httprate.WithLimitHandler(rateLimitedHandler),
		httprate.WithErrorHandler(rateLimiterErrorHandler),
	}
	if rdb != nil {
		options = append(options, httprateredis.WithRedisLimitCounter(&httprateredis.Config{
			Client:       rdb,
			PrefixKey:    "rl:" + tier,
			WindowLength: window,
			OnFallbackChange: func(activated bool) {
				if activated {
					log.Printf("rate limiter [%s]: redis unavailable, using local in-memory fallback", tier)
				} else {
					log.Printf("rate limiter [%s]: redis reconnected", tier)
				}
			},
		}))
	}
	return httprate.LimitBy(requests, window, key, options...)
}

// Close releases the shared Redis client. Safe to call when Redis is unused.
func (rl *RateLimiters) Close() error {
	if rl != nil && rl.redis != nil {
		return rl.redis.Close()
	}
	return nil
}
