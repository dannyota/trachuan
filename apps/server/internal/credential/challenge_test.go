package credential

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestChallenge_MissingInvalidInsufficient(t *testing.T) {
	v, _, _, plain := fixture(t)
	for _, tc := range []struct {
		name    string
		headers []string
		status  int
		want    string
	}{{"missing", nil, 401, `Bearer resource_metadata="https://example.invalid/.well-known/oauth-protected-resource/mcp", scope="mcp:read"`}, {"invalid", []string{"Bearer invalid"}, 401, `Bearer resource_metadata="https://example.invalid/.well-known/oauth-protected-resource/mcp", scope="mcp:read", error="invalid_token"`}, {"valid", []string{"Bearer " + plain}, 204, ""}} {
		t.Run(tc.name, func(t *testing.T) {
			w := response(v, tc.headers)
			if w.Code != tc.status || w.Header().Get("WWW-Authenticate") != tc.want {
				t.Errorf("status %d challenge %q", w.Code, w.Header().Get("WWW-Authenticate"))
			}
		})
	}
	w := httptest.NewRecorder()
	InsufficientScope(w, "https://example.test/mcp")
	if w.Code != http.StatusForbidden || w.Header().Get("WWW-Authenticate") != `Bearer resource_metadata="https://example.test/.well-known/oauth-protected-resource/mcp", scope="mcp:read", error="insufficient_scope"` {
		t.Fatalf("status %d challenge %q", w.Code, w.Header().Get("WWW-Authenticate"))
	}
}
