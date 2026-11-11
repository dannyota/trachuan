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
			unauthorized(w, v.Audience, hasBearer(headers))
			return
		}
		p, err := v.Verify(r.Context(), headers[0][7:])
		if errors.Is(err, ErrUnavailable) {
			http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
			return
		}
		if errors.Is(err, ErrInsufficientScope) {
			InsufficientScope(w, v.Audience)
			return
		}
		if err != nil {
			unauthorized(w, v.Audience, true)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), principalKey{}, p)))
	})
}

func hasBearer(headers []string) bool {
	for _, header := range headers {
		if len(header) > 6 && strings.EqualFold(header[:6], "Bearer") && (header[6] == ' ' || header[6] == '\t') {
			return true
		}
	}
	return false
}

func unauthorized(w http.ResponseWriter, audience string, invalid bool) {
	challenge := challengeValue(audience)
	if invalid {
		challenge += `, error="invalid_token"`
	}
	w.Header().Set("WWW-Authenticate", challenge)
	http.Error(w, "Unauthorized", http.StatusUnauthorized)
}

// InsufficientScope writes the challenge for a valid credential lacking MCP scope.
func InsufficientScope(w http.ResponseWriter, audience string) {
	w.Header().Set("WWW-Authenticate", challengeValue(audience)+`, error="insufficient_scope"`)
	http.Error(w, "Forbidden", http.StatusForbidden)
}

func challengeValue(audience string) string {
	base := strings.TrimSuffix(audience, "/mcp")
	return `Bearer resource_metadata="` + base + `/.well-known/oauth-protected-resource/mcp", scope="mcp:read"`
}
