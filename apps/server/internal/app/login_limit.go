package app

import (
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"time"

	"danny.vn/trachuan/apps/server/internal/apierror"
)

type loginBucket struct {
	credit  time.Duration
	updated time.Time
}
type loginLimiter struct {
	mu      sync.Mutex
	clock   func() time.Time
	proxies []netip.Prefix
	buckets map[string]loginBucket
}

func newLoginLimiter(clock func() time.Time, proxies []netip.Prefix) *loginLimiter {
	return &loginLimiter{clock: clock, proxies: proxies, buckets: make(map[string]loginBucket)}
}
func (l *loginLimiter) trusted(ip netip.Addr) bool {
	for _, p := range l.proxies {
		if p.Contains(ip.Unmap()) {
			return true
		}
	}
	return false
}
func (l *loginLimiter) clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return "unknown"
	}
	peer, err := netip.ParseAddr(host)
	if err != nil {
		return "unknown"
	}
	peer = peer.Unmap()
	if !l.trusted(peer) {
		return peer.String()
	}
	values := strings.Split(strings.Join(r.Header.Values("X-Forwarded-For"), ","), ",")
	for i := len(values) - 1; i >= 0; i-- {
		ip, err := netip.ParseAddr(strings.TrimSpace(values[i]))
		if err != nil {
			return peer.String()
		}
		ip = ip.Unmap()
		if !l.trusted(ip) {
			return ip.String()
		}
	}
	return peer.String()
}
func (l *loginLimiter) allow(ip string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.clock()
	b, ok := l.buckets[ip]
	if !ok {
		if len(l.buckets) >= 100000 {
			for k, v := range l.buckets {
				if now.Sub(v.updated) >= time.Minute {
					delete(l.buckets, k)
				}
			}
			if len(l.buckets) >= 100000 {
				return false, 3 * time.Second
			}
		}
		b = loginBucket{credit: time.Minute, updated: now}
	}
	if elapsed := now.Sub(b.updated); elapsed > 0 {
		b.credit = min(time.Minute, b.credit+elapsed)
		b.updated = now
	}
	if b.credit < 3*time.Second {
		l.buckets[ip] = b
		return false, 3*time.Second - b.credit
	}
	b.credit -= 3 * time.Second
	l.buckets[ip] = b
	return true, 0
}
func (l *loginLimiter) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ok, wait := l.allow(l.clientIP(r)); !ok {
			apierror.RateLimited(w, wait)
			return
		}
		next.ServeHTTP(w, r)
	})
}
