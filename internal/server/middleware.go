package server

import (
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/nkh/rdw/internal/auth"
)

// apiError writes a JSON error response.
func apiError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// ----------------------------------------------------------------------------
// Token auth middleware
// ----------------------------------------------------------------------------

// tokenKey is the context key used to store the authenticated token.
type tokenKey struct{}

// authMiddleware validates the Bearer token in the Authorization header.
// Requests from loopback with no token are allowed when noAuth is true.
// Owner requests authenticated via Unix socket set tokenID to "".
func authMiddleware(store *auth.Store, noAuth bool, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if noAuth {
			next.ServeHTTP(w, r)
			return
		}

		header := r.Header.Get("Authorization")
		if header == "" {
			apiError(w, http.StatusUnauthorized, "authorization header required")
			return
		}

		if !strings.HasPrefix(header, "Bearer ") {
			apiError(w, http.StatusUnauthorized, "bearer token required")
			return
		}

		plain := strings.TrimPrefix(header, "Bearer ")
		tok, ok := store.Verify(plain)

		if !ok {
			apiError(w, http.StatusUnauthorized, "invalid or expired token")
			return
		}

		// Attach token to context for downstream handlers.
		ctx := r.Context()
		_ = tok // will be stored in context in phase 3 when scope checks land
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// ----------------------------------------------------------------------------
// Loopback guard
// ----------------------------------------------------------------------------

// loopbackOnly rejects requests that do not originate from loopback.
func loopbackOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			host = r.RemoteAddr
		}

		ip := net.ParseIP(host)
		if ip == nil || !ip.IsLoopback() {
			http.Error(w, "admin console is restricted to loopback", http.StatusForbidden)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// ----------------------------------------------------------------------------
// Rate limiter (token bucket per source IP)
// ----------------------------------------------------------------------------

const (
	rateLimitRequests = 10
	rateLimitWindow   = time.Minute
)

type rateBucket struct {
	count    int
	resetAt  time.Time
}

// RateLimiter is a simple per-IP request rate limiter.
type RateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*rateBucket
}

// NewRateLimiter creates a rate limiter.
func NewRateLimiter() *RateLimiter {
	return &RateLimiter{buckets: make(map[string]*rateBucket)}
}

// Allow returns true if the request from ip should be permitted.
func (rl *RateLimiter) Allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	b, ok := rl.buckets[ip]

	if !ok || now.After(b.resetAt) {
		rl.buckets[ip] = &rateBucket{count: 1, resetAt: now.Add(rateLimitWindow)}
		return true
	}

	if b.count >= rateLimitRequests {
		return false
	}

	b.count++
	return true
}

// rateLimitMiddleware applies rl to requests where the Authorization header
// is absent (unauthenticated endpoints only).
func rateLimitMiddleware(rl *RateLimiter, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			host, _, err := net.SplitHostPort(r.RemoteAddr)
			if err != nil {
				host = r.RemoteAddr
			}

			if !rl.Allow(host) {
				apiError(w, http.StatusTooManyRequests, "rate limit exceeded")
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}

// ----------------------------------------------------------------------------
// CORS / content-type helpers
// ----------------------------------------------------------------------------

// jsonResponse writes a JSON-encoded value with status 200.
func jsonResponse(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
