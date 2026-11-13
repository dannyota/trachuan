//go:build e2e

package e2e

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

// Account tokens keep corpus checks out of the OAuth login bucket.
func personalToken(t *testing.T, c *http.Client, server string) string {
	t.Helper()
	var account struct {
		CSRF string `json:"csrf_token"`
	}
	oauthJSON(t, get(t, c, server+"/api/v1/me"), http.StatusOK, &account)
	if account.CSRF == "" {
		t.Fatal("missing account CSRF token")
	}
	req, err := http.NewRequest(http.MethodPost, server+"/api/v1/me/tokens", strings.NewReader(`{"name":"MCP search fixture"}`))
	if err != nil {
		t.Fatal("token creation request failed")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", server)
	req.Header.Set("X-CSRF-Token", account.CSRF)
	response, err := c.Do(req)
	if err != nil {
		t.Fatal("token creation request failed")
	}
	var result struct {
		Token string `json:"token"`
	}
	oauthJSON(t, response, http.StatusCreated, &result)
	if result.Token == "" {
		t.Fatal("missing personal token")
	}
	return result.Token
}

type personalTokenTransport func(*http.Request) (*http.Response, error)

func (transport personalTokenTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	return transport(r)
}

func TestPersonalToken_UsesAccountAPI(t *testing.T) {
	const server = "http://example.invalid"
	calls := 0
	c := &http.Client{Transport: personalTokenTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		status, body := http.StatusInternalServerError, `{}`
		switch {
		case calls == 1 && r.Method == http.MethodGet && r.URL.Path == "/api/v1/me":
			status, body = http.StatusOK, `{"csrf_token":"fixture-csrf"}`
		case calls == 2 && r.Method == http.MethodPost && r.URL.Path == "/api/v1/me/tokens":
			if r.Header.Get("Origin") != server || r.Header.Get("X-CSRF-Token") != "fixture-csrf" || r.Header.Get("Content-Type") != "application/json" {
				t.Error("missing token creation headers")
			}
			var input struct {
				Name string `json:"name"`
			}
			if err := json.NewDecoder(r.Body).Decode(&input); err != nil || input.Name != "MCP search fixture" {
				t.Error("invalid token creation body")
			}
			status, body = http.StatusCreated, `{"token":"trp_fixture"}`
		default:
			t.Errorf("unexpected credential request %d: %s %s", calls, r.Method, r.URL.Path)
		}
		return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	if token := personalToken(t, c, server); token != "trp_fixture" {
		t.Fatal("missing personal token")
	}
	if calls != 2 {
		t.Fatalf("credential requests %d, want 2", calls)
	}
}
