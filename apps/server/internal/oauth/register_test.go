package oauth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRegister_FieldsResponseAndErrors(t *testing.T) {
	const redirects = `"redirect_uris":["http://127.0.0.1/cb"]`
	for _, tc := range []struct {
		name, body, code string
		status           int
	}{
		{"defaults", `{` + redirects + `}`, "", 201},
		{"fields", `{` + redirects + `,"client_name":"Client","grant_types":["authorization_code"],"response_types":["code"],"scope":"mcp:read","token_endpoint_auth_method":"none","logo_uri":"https://ignored.example/logo","application_type":"ignored"}`, "", 201},
		{"ignored public keys", `{` + redirects + `,"jwks":{"keys":[]},"jwks_uri":"https://ignored.example/keys"}`, "", 201},
		{"missing redirects", `{}`, "invalid_redirect_uri", 400},
		{"bad redirect", `{"redirect_uris":["http://example.com/cb"]}`, "invalid_redirect_uri", 400},
		{"redirect type", `{"redirect_uris":"wrong"}`, "invalid_redirect_uri", 400},
		{"empty redirects", `{"redirect_uris":[]}`, "invalid_redirect_uri", 400},
		{"too many redirects", `{"redirect_uris":["https://a.example/1","https://a.example/2","https://a.example/3","https://a.example/4","https://a.example/5","https://a.example/6"]}`, "invalid_redirect_uri", 400},
		{"name type", `{` + redirects + `,"client_name":null}`, "invalid_client_metadata", 400},
		{"secret method", `{` + redirects + `,"token_endpoint_auth_method":"client_secret_basic"}`, "invalid_client_metadata", 400},
		{"secret", `{` + redirects + `,"client_secret":"refused"}`, "invalid_client_metadata", 400},
		{"plural secret", `{` + redirects + `,"token_endpoint_auth_methods_supported":["none","private_key_jwt"]}`, "invalid_client_metadata", 400},
		{"grant", `{` + redirects + `,"grant_types":["client_credentials"]}`, "invalid_client_metadata", 400},
		{"empty grant", `{` + redirects + `,"grant_types":[]}`, "invalid_client_metadata", 400},
		{"response", `{` + redirects + `,"response_types":["token"]}`, "invalid_client_metadata", 400},
		{"scope", `{` + redirects + `,"scope":"other"}`, "invalid_client_metadata", 400},
		{"duplicate", `{` + redirects + `,"scope":"mcp:read","scope":"other"}`, "invalid_client_metadata", 400},
		{"trailing", `{` + redirects + `} {}`, "invalid_client_metadata", 400},
		{"oversize", `{` + redirects + `,"ignored":"` + strings.Repeat("x", 4096) + `"}`, "invalid_client_metadata", 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stored := false
			h := NewRegisterHandler(func(_ context.Context, c Client) (Client, error) {
				stored = true
				c.CreatedAt = time.Unix(1794355200, 0)
				return c, nil
			})
			r := httptest.NewRequest("POST", "/oauth/register", strings.NewReader(tc.body))
			r.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			var got map[string]any
			if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if w.Code != tc.status {
				t.Fatalf("status %d, body %s", w.Code, w.Body)
			}
			if w.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("response cacheable")
			}
			if tc.status != 201 {
				if got["error"] != tc.code || stored {
					t.Fatalf("error %v stored %v", got, stored)
				}
				return
			}
			if !stored || got["client_id_issued_at"] != float64(1794355200) || len(got["client_id"].(string)) != 43 {
				t.Fatalf("bad issued fields: %v", got)
			}
			if got["token_endpoint_auth_method"] != "none" || got["scope"] != "mcp:read" {
				t.Fatalf("bad metadata: %v", got)
			}
			if strings.Join(toStrings(t, got["grant_types"]), ",") != "authorization_code,refresh_token" || strings.Join(toStrings(t, got["response_types"]), ",") != "code" {
				t.Fatal("effective grants differ")
			}
			if len(got) != 8 || got["client_name"] == nil || len(toStrings(t, got["redirect_uris"])) != 1 {
				t.Fatalf("unexpected fields: %v", got)
			}
		})
	}
	for _, tc := range []struct {
		method, content string
		status          int
	}{{"GET", "application/json", 405}, {"POST", "text/plain", 415}} {
		h := NewRegisterHandler(func(context.Context, Client) (Client, error) { t.Fatal("invalid request stored"); return Client{}, nil })
		r := httptest.NewRequest(tc.method, "/oauth/register", strings.NewReader(`{`+redirects+`}`))
		r.Header.Set("Content-Type", tc.content)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Fatalf("status %d", w.Code)
		}
	}
	h := NewRegisterHandler(func(context.Context, Client) (Client, error) {
		return Client{}, errors.New("database detail must stay private")
	})
	r := httptest.NewRequest("POST", "/oauth/register", strings.NewReader(`{`+redirects+`}`))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 503 || !strings.Contains(w.Body.String(), `"temporarily_unavailable"`) || strings.Contains(w.Body.String(), "database detail") {
		t.Fatalf("unavailable = %d %s", w.Code, w.Body)
	}
}

func toStrings(t *testing.T, value any) []string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	if err = json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}
