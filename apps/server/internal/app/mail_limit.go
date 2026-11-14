package app

import (
	"net/http"
	"sync"
	"time"

	"danny.vn/trachuan/apps/server/internal/apierror"
)

type mailLimiter struct {
	mu       sync.Mutex
	source   *loginLimiter
	attempts map[string][]time.Time
}

func newMailLimiter(source *loginLimiter) *mailLimiter {
	return &mailLimiter{source: source, attempts: make(map[string][]time.Time)}
}
func (l *mailLimiter) allow(ip string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.source.clock()
	cutoff := now.Add(-time.Hour)
	if _, ok := l.attempts[ip]; !ok && len(l.attempts) >= 100000 {
		for key, at := range l.attempts {
			if !at[len(at)-1].After(cutoff) {
				delete(l.attempts, key)
			}
		}
		if len(l.attempts) >= 100000 {
			return false, time.Hour
		}
	}
	at := l.attempts[ip]
	for len(at) > 0 && !at[0].After(cutoff) {
		at = at[1:]
	}
	if len(at) >= 5 {
		l.attempts[ip] = at
		return false, at[0].Add(time.Hour).Sub(now)
	}
	l.attempts[ip] = append(at, now)
	return true, 0
}
func (l *mailLimiter) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ok, wait := l.allow(l.source.clientIP(r)); !ok {
			apierror.RateLimited(w, wait)
			return
		}
		next.ServeHTTP(w, r)
	})
}
