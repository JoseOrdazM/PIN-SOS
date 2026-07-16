// Package middleware provides HTTP middleware for PIN-SOS.
package middleware

import (
	"log"
	"net"
	"net/http"
	"sync"
	"time"
)

// RateLimiter implements a per-IP token bucket plus a global cap.
// Stdlib-only, no external dependencies.
type RateLimiter struct {
	mu       sync.Mutex
	buckets  map[string]*bucket
	perIP    int           // tokens per window per IP
	global   int           // tokens per window for everyone combined
	window   time.Duration // refill window
	globalN  int
	globalAt time.Time
}

type bucket struct {
	n  int
	at time.Time
}

// NewRateLimiter allows perIP requests per IP and global requests in total
// within each window. Example: NewRateLimiter(5, 60, time.Minute).
func NewRateLimiter(perIP, global int, window time.Duration) *RateLimiter {
	rl := &RateLimiter{
		buckets: make(map[string]*bucket),
		perIP:   perIP,
		global:  global,
		window:  window,
	}
	// Periodic cleanup so the map does not grow forever.
	go func() {
		t := time.NewTicker(10 * time.Minute)
		defer t.Stop()
		for range t.C {
			rl.mu.Lock()
			cutoff := time.Now().Add(-2 * rl.window)
			for ip, b := range rl.buckets {
				if b.at.Before(cutoff) {
					delete(rl.buckets, ip)
				}
			}
			rl.mu.Unlock()
		}
	}()
	return rl
}

// clientIP prefers Fly-Client-IP / X-Forwarded-For (set by the Fly proxy),
// falling back to the socket address.
func clientIP(r *http.Request) string {
	if ip := r.Header.Get("Fly-Client-IP"); ip != "" {
		return ip
	}
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// First hop is the original client.
		for i := 0; i < len(xff); i++ {
			if xff[i] == ',' {
				return xff[:i]
			}
		}
		return xff
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// Allow reports whether this request fits within the limits.
func (rl *RateLimiter) Allow(r *http.Request) bool {
	ip := clientIP(r)
	now := time.Now()

	rl.mu.Lock()
	defer rl.mu.Unlock()

	// Global window.
	if now.Sub(rl.globalAt) > rl.window {
		rl.globalN = 0
		rl.globalAt = now
	}
	if rl.globalN >= rl.global {
		return false
	}

	// Per-IP window.
	b := rl.buckets[ip]
	if b == nil || now.Sub(b.at) > rl.window {
		b = &bucket{at: now}
		rl.buckets[ip] = b
	}
	if b.n >= rl.perIP {
		return false
	}

	b.n++
	rl.globalN++
	return true
}

// Middleware wraps a handler and returns 429 when limits are exceeded.
func (rl *RateLimiter) Middleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !rl.Allow(r) {
			log.Printf("rate limited: %s %s from %s", r.Method, r.URL.Path, clientIP(r))
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Retry-After", "60")
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"ok":false,"error":"Demasiadas solicitudes. Espera un minuto e intenta de nuevo."}`))
			return
		}
		next(w, r)
	}
}
