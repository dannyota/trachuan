package app

import (
	"net/http"
	"net/netip"
)

func (l *loginLimiter) publicMiddleware(next http.Handler) http.Handler {
	limited := l.middleware(next)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		peer, err := netip.ParseAddrPort(r.RemoteAddr)
		_, forwarded := r.Header["X-Forwarded-For"]
		if err == nil && peer.Addr().Unmap().IsLoopback() && !forwarded {
			next.ServeHTTP(w, r)
			return
		}
		limited.ServeHTTP(w, r)
	})
}
