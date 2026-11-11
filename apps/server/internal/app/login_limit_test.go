package app

import (
	"net/http/httptest"
	"net/netip"
	"testing"
	"time"
)

func TestLoginLimit_ClientAndBudget(t *testing.T) {
	now := time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC)
	limiter := newLoginLimiter(func() time.Time { return now }, []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")})
	for _, tt := range []struct{ peer, forwarded, want string }{
		{"192.0.2.1:123", "198.51.100.1", "192.0.2.1"},
		{"10.0.0.1:123", "198.51.100.1, 192.0.2.1, 10.0.0.2", "192.0.2.1"},
		{"10.0.0.1:123", "invalid", "10.0.0.1"},
	} {
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = tt.peer
		r.Header.Set("X-Forwarded-For", tt.forwarded)
		if got := limiter.clientIP(r); got != tt.want {
			t.Errorf("client = %q, want %q", got, tt.want)
		}
	}
	for i := 0; i < 20; i++ {
		if ok, _ := limiter.allow("192.0.2.1"); !ok {
			t.Fatalf("request %d rejected", i+1)
		}
	}
	if ok, _ := limiter.allow("192.0.2.1"); ok {
		t.Fatal("21st request accepted")
	}
	now = now.Add(3 * time.Second)
	if ok, _ := limiter.allow("192.0.2.1"); !ok {
		t.Fatal("budget did not refill")
	}
}
