package credential

import (
	"context"
	"errors"
	"net/http"
	"strings"
)

type principalKey struct{}

// FromContext returns the principal attached by Middleware.
func FromContext(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(principalKey{}).(Principal)
	return p, ok
}

// Middleware requires exactly one bearer credential and adds its principal to
// the request context. Cookies do not authorize MCP requests.
func Middleware(v *Verifier, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headers := r.Header.Values("Authorization")
		if len(headers) != 1 || len(headers[0]) < 7 || !strings.EqualFold(headers[0][:7], "Bearer ") {
			unauthorized(w)
			return
		}
		p, err := v.Verify(r.Context(), headers[0][7:])
		if errors.Is(err, ErrUnavailable) {
			http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
			return
		}
		if err != nil {
			unauthorized(w)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), principalKey{}, p)))
	})
}

func unauthorized(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", `Bearer error="invalid_token"`)
	http.Error(w, "Unauthorized", http.StatusUnauthorized)
}
