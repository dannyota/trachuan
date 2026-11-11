package oauth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestMetadata_AuthorizationServer(t *testing.T) {
	w := httptest.NewRecorder()
	AuthorizationServerMetadata("https://example.test").ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
	var got map[string]any
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &got) != nil {
		t.Fatalf("response %d %s", w.Code, w.Body.String())
	}
	want := map[string]any{"issuer": "https://example.test", "authorization_endpoint": "https://example.test/oauth/authorize", "token_endpoint": "https://example.test/oauth/token", "registration_endpoint": "https://example.test/oauth/register", "revocation_endpoint": "https://example.test/oauth/revoke", "code_challenge_methods_supported": []any{"S256"}, "token_endpoint_auth_methods_supported": []any{"none"}, "revocation_endpoint_auth_methods_supported": []any{"none"}, "client_id_metadata_document_supported": true, "authorization_response_iss_parameter_supported": true}
	for k, v := range want {
		if !reflect.DeepEqual(got[k], v) {
			t.Errorf("%s = %v, want %v", k, got[k], v)
		}
	}
}
func TestMetadata_ProtectedResourceBothPaths(t *testing.T) {
	for _, path := range []string{"/.well-known/oauth-protected-resource/mcp", "/.well-known/oauth-protected-resource"} {
		t.Run(path, func(t *testing.T) {
			w := httptest.NewRecorder()
			ProtectedResourceMetadata("https://example.test").ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
			var got map[string]any
			if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &got) != nil || got["resource"] != "https://example.test/mcp" || !reflect.DeepEqual(got["authorization_servers"], []any{"https://example.test"}) || !reflect.DeepEqual(got["scopes_supported"], []any{"mcp:read"}) {
				t.Fatalf("response %d %s", w.Code, w.Body.String())
			}
		})
	}
}
