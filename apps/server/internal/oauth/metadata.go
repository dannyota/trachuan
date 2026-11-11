// Package oauth provides OAuth protocol helpers and metadata handlers.
package oauth

import (
	"encoding/json"
	"net/http"
)

// AuthorizationServerMetadata serves metadata for the configured issuer.
func AuthorizationServerMetadata(base string) http.Handler {
	return metadata(map[string]any{
		"issuer": base, "authorization_endpoint": base + "/oauth/authorize", "token_endpoint": base + "/oauth/token", "registration_endpoint": base + "/oauth/register", "revocation_endpoint": base + "/oauth/revoke",
		"response_types_supported": []string{"code"}, "grant_types_supported": []string{"authorization_code", "refresh_token"}, "code_challenge_methods_supported": []string{"S256"}, "token_endpoint_auth_methods_supported": []string{"none"}, "revocation_endpoint_auth_methods_supported": []string{"none"}, "scopes_supported": []string{"mcp:read"}, "client_id_metadata_document_supported": true, "authorization_response_iss_parameter_supported": true,
	})
}

// ProtectedResourceMetadata serves the MCP resource metadata at either discovery path.
func ProtectedResourceMetadata(base string) http.Handler {
	return metadata(map[string]any{"resource": base + "/mcp", "authorization_servers": []string{base}, "scopes_supported": []string{"mcp:read"}, "bearer_methods_supported": []string{"header"}})
}
func metadata(document map[string]any) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "public, max-age=3600")
		_ = json.NewEncoder(w).Encode(document)
	})
}
