package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRateLimiterPerIP(t *testing.T) {
	rl := NewRateLimiter(3, 100, time.Minute)
	h := rl.Middleware(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := func(ip string) int {
		r := httptest.NewRequest("POST", "/api/alert", nil)
		r.Header.Set("Fly-Client-IP", ip)
		rec := httptest.NewRecorder()
		h(rec, r)
		return rec.Code
	}

	for i := 0; i < 3; i++ {
		if code := req("1.2.3.4"); code != http.StatusOK {
			t.Fatalf("request %d: want 200, got %d", i+1, code)
		}
	}
	if code := req("1.2.3.4"); code != http.StatusTooManyRequests {
		t.Fatalf("4th request same IP: want 429, got %d", code)
	}
	// A different IP is unaffected.
	if code := req("5.6.7.8"); code != http.StatusOK {
		t.Fatalf("other IP: want 200, got %d", code)
	}
}

func TestRateLimiterGlobal(t *testing.T) {
	rl := NewRateLimiter(100, 5, time.Minute)
	ok := 0
	for i := 0; i < 10; i++ {
		r := httptest.NewRequest("POST", "/", nil)
		r.Header.Set("Fly-Client-IP", string(rune('a'+i))+".ip")
		if rl.Allow(r) {
			ok++
		}
	}
	if ok != 5 {
		t.Fatalf("global cap: want 5 allowed, got %d", ok)
	}
}
