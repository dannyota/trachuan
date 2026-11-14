//go:build e2e

package e2e

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

const oauthRedirect = "http://127.0.0.1:12345/callback"
const oauthVerifier = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
const oauthState = "oauth-e2e-state"
const oauthClientName = "OAuth e2e fixture"

type oauthTransport struct{ base http.RoundTripper }

func (tr oauthTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	copyRequest := req.Clone(req.Context())
	copyRequest.Header.Set("X-Forwarded-For", "192.0.2.42")
	return tr.base.RoundTrip(copyRequest)
}

type oauthTokens struct {
	Access  string `json:"access_token"`
	Refresh string `json:"refresh_token"`
	Type    string `json:"token_type"`
	Expires int    `json:"expires_in"`
}

func oauthJSON(t *testing.T, r *http.Response, status int, out any) {
	t.Helper()
	defer closeBody(r)
	if r.StatusCode != status {
		t.Fatalf("response status %d, want %d", r.StatusCode, status)
	}
	if err := json.NewDecoder(r.Body).Decode(out); err != nil {
		t.Fatal("invalid JSON response")
	}
}

func oauthPostForm(t *testing.T, c *http.Client, target string, form url.Values) *http.Response {
	t.Helper()
	r, err := c.PostForm(target, form)
	if err != nil {
		t.Fatal("form request failed")
	}
	return r
}

func oauthLocation(t *testing.T, r *http.Response, status int) *url.URL {
	t.Helper()
	defer closeBody(r)
	if r.StatusCode != status {
		t.Fatalf("redirect status %d, want %d", r.StatusCode, status)
	}
	loc, err := r.Location()
	if err != nil {
		t.Fatal("invalid redirect")
	}
	return loc
}

func oauthMetadata(t *testing.T, c *http.Client, server string) {
	t.Helper()
	var resource struct {
		Resource string   `json:"resource"`
		Servers  []string `json:"authorization_servers"`
		Scopes   []string `json:"scopes_supported"`
	}
	oauthJSON(t, get(t, c, server+"/.well-known/oauth-protected-resource/mcp"), 200, &resource)
	if resource.Resource != server+"/mcp" || !slices.Contains(resource.Servers, server) || !slices.Contains(resource.Scopes, "mcp:read") {
		t.Fatal("invalid protected resource metadata")
	}
	var auth struct {
		Issuer    string   `json:"issuer"`
		Authorize string   `json:"authorization_endpoint"`
		Token     string   `json:"token_endpoint"`
		Register  string   `json:"registration_endpoint"`
		Revoke    string   `json:"revocation_endpoint"`
		PKCE      []string `json:"code_challenge_methods_supported"`
	}
	oauthJSON(t, get(t, c, server+"/.well-known/oauth-authorization-server"), 200, &auth)
	if auth.Issuer != server || auth.Authorize != server+"/oauth/authorize" || auth.Token != server+"/oauth/token" || auth.Register != server+"/oauth/register" || auth.Revoke != server+"/oauth/revoke" || !slices.Contains(auth.PKCE, "S256") {
		t.Fatal("invalid authorization server metadata")
	}
	r := oauthMCP(t, c, server, "", `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	defer closeBody(r)
	if r.StatusCode != http.StatusUnauthorized || !strings.Contains(r.Header.Get("WWW-Authenticate"), server+"/.well-known/oauth-protected-resource/mcp") {
		t.Fatal("missing unauthenticated MCP challenge")
	}
}

func oauthRegister(t *testing.T, c *http.Client, server string) string {
	t.Helper()
	r, err := c.Post(server+"/oauth/register", "application/json", strings.NewReader(`{"client_name":"OAuth e2e fixture","redirect_uris":["http://127.0.0.1:12345/callback"],"grant_types":["authorization_code","refresh_token"],"response_types":["code"],"token_endpoint_auth_method":"none"}`))
	if err != nil {
		t.Fatal("registration request failed")
	}
	var registration struct {
		Client string `json:"client_id"`
	}
	oauthJSON(t, r, http.StatusCreated, &registration)
	if registration.Client == "" {
		t.Fatal("missing client ID")
	}
	return registration.Client
}

func oauthAuthorize(t *testing.T, c *http.Client, server, id string, signedOut bool) oauthTokens {
	t.Helper()
	q := url.Values{"client_id": {id}, "redirect_uri": {oauthRedirect}, "response_type": {"code"}, "code_challenge": {oauth2.S256ChallengeFromVerifier(oauthVerifier)}, "code_challenge_method": {"S256"}, "state": {oauthState}, "resource": {server + "/mcp"}, "scope": {"mcp:read"}}
	loc := oauthLocation(t, get(t, c, server+"/oauth/authorize?"+q.Encode()), http.StatusSeeOther)
	if !strings.HasPrefix(loc.Path, "/oauth/consent/") {
		t.Fatal("missing consent redirect")
	}
	consentPath := loc.Path
	if signedOut {
		loc = oauthLocation(t, get(t, c, server+consentPath), http.StatusSeeOther)
		if loc.Path != "/login" || loc.Query().Get("next") != consentPath {
			t.Fatal("missing sign-in page redirect")
		}
		page := get(t, c, server+loc.RequestURI())
		if page.StatusCode != http.StatusOK {
			t.Fatal("sign-in page unavailable")
		}
		closeBody(page)
		loc = oauthLocation(t, get(t, c, server+"/api/v1/auth/google/start?next="+url.QueryEscape(consentPath)), http.StatusFound)
		google := base(t, "E2E_GOOGLE_URL")
		if !strings.HasPrefix(loc.String(), google+"/") {
			t.Fatal("wrong Google login destination")
		}
		form := fields(t, get(t, c, loc.String()))
		form.Set("email", "oauth-e2e@example.invalid")
		form.Set("email_verified", "on")
		loc = oauthLocation(t, oauthPostForm(t, c, google+"/authorize", form), http.StatusFound)
		if !strings.HasPrefix(loc.String(), server+"/api/v1/auth/google/callback?") {
			t.Fatal("wrong Google callback")
		}
		loc = oauthLocation(t, get(t, c, loc.String()), http.StatusFound)
		if loc.String() != server+consentPath {
			t.Fatal("login did not return to consent")
		}
	}
	form := fields(t, get(t, c, server+consentPath))
	if form.Get("csrf_token") == "" {
		t.Fatal("missing consent CSRF token")
	}
	form.Set("decision", "approve")
	req, err := http.NewRequest(http.MethodPost, server+consentPath, strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatal("consent request failed")
	}
	req.Header.Set("Origin", server)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r, err := c.Do(req)
	if err != nil {
		t.Fatal("consent request failed")
	}
	loc = oauthLocation(t, r, http.StatusSeeOther)
	if loc.Scheme+"://"+loc.Host+loc.Path != oauthRedirect || loc.Query().Get("code") == "" || loc.Query().Get("state") != oauthState || loc.Query().Get("iss") != server {
		t.Fatal("invalid authorization response")
	}
	return oauthToken(t, c, server, url.Values{"grant_type": {"authorization_code"}, "client_id": {id}, "redirect_uri": {oauthRedirect}, "code": {loc.Query().Get("code")}, "code_verifier": {oauthVerifier}, "resource": {server + "/mcp"}})
}

func oauthToken(t *testing.T, c *http.Client, server string, form url.Values) oauthTokens {
	t.Helper()
	var tokens oauthTokens
	oauthJSON(t, oauthPostForm(t, c, server+"/oauth/token", form), http.StatusOK, &tokens)
	if !strings.HasPrefix(tokens.Access, "tra_") || !strings.HasPrefix(tokens.Refresh, "trr_") || !strings.EqualFold(tokens.Type, "Bearer") || tokens.Expires <= 0 {
		t.Fatal("invalid token response")
	}
	return tokens
}

func oauthMCP(t *testing.T, c *http.Client, server, token, body string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, server+"/mcp", strings.NewReader(body))
	if err != nil {
		t.Fatal("MCP request failed")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	r, err := c.Do(req)
	if err != nil {
		t.Fatal("MCP request failed")
	}
	return r
}

func oauthRPC(t *testing.T, c *http.Client, server, token, body string, result any) {
	t.Helper()
	var rpc struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      int             `json:"id"`
		Result  json.RawMessage `json:"result"`
		Error   json.RawMessage `json:"error"`
	}
	oauthJSON(t, oauthMCP(t, c, server, token, body), http.StatusOK, &rpc)
	if rpc.JSONRPC != "2.0" || rpc.ID != 1 || len(rpc.Result) == 0 || len(rpc.Error) != 0 || json.Unmarshal(rpc.Result, result) != nil {
		t.Fatal("invalid MCP response")
	}
}

func oauthMCPWorks(t *testing.T, c *http.Client, server, token string) {
	t.Helper()
	var init struct {
		Protocol string `json:"protocolVersion"`
		Server   struct {
			Name string `json:"name"`
		} `json:"serverInfo"`
	}
	oauthRPC(t, c, server, token, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"e2e","version":"1"}}}`, &init)
	if init.Protocol == "" || init.Server.Name == "" {
		t.Fatal("invalid MCP initialization")
	}
	var list struct {
		Tools []struct {
			Name string `json:"name"`
		} `json:"tools"`
	}
	oauthRPC(t, c, server, token, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`, &list)
	names := make([]string, 0, len(list.Tools))
	for _, tool := range list.Tools {
		names = append(names, tool.Name)
	}
	for _, name := range []string{"guide", "corpus_status", "search", "read", "taxonomy"} {
		if !slices.Contains(names, name) {
			t.Fatalf("missing MCP tool %s", name)
		}
	}
	var status struct {
		IsError    bool `json:"isError"`
		Structured struct {
			Release   string `json:"release"`
			Documents int    `json:"documents"`
		} `json:"structuredContent"`
	}
	oauthRPC(t, c, server, token, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"corpus_status","arguments":{"country":"vn"}}}`, &status)
	if status.IsError || status.Structured.Release == "" || status.Structured.Documents < 1 {
		t.Fatal("invalid corpus status")
	}
}

func oauthWaitRevoked(t *testing.T, c *http.Client, server, token string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		r := oauthMCP(t, c, server, token, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
		status := r.StatusCode
		closeBody(r)
		if status == http.StatusUnauthorized {
			return
		}
		if status != http.StatusOK || time.Now().After(deadline) {
			t.Fatalf("revoked MCP status %d", status)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

type oauthGrant struct {
	ID   int64  `json:"id"`
	Name string `json:"client_name"`
}

func oauthGrants(t *testing.T, c *http.Client, server string) []oauthGrant {
	t.Helper()
	var grants []oauthGrant
	oauthJSON(t, get(t, c, server+"/api/v1/me/grants"), http.StatusOK, &grants)
	if grants == nil {
		t.Fatal("invalid grants response")
	}
	return grants
}

func oauthGrantID(t *testing.T, c *http.Client, server string) int64 {
	t.Helper()
	var id int64
	for _, grant := range oauthGrants(t, c, server) {
		if grant.Name == oauthClientName {
			if id != 0 || grant.ID <= 0 {
				t.Fatal("ambiguous grant")
			}
			id = grant.ID
		}
	}
	if id == 0 {
		t.Fatal("missing grant")
	}
	return id
}

func oauthDeleteGrant(t *testing.T, c *http.Client, server string) {
	t.Helper()
	var account struct {
		CSRF string `json:"csrf_token"`
	}
	oauthJSON(t, get(t, c, server+"/api/v1/me"), http.StatusOK, &account)
	if account.CSRF == "" {
		t.Fatal("missing account CSRF token")
	}
	id := oauthGrantID(t, c, server)
	req, err := http.NewRequest(http.MethodDelete, fmt.Sprintf("%s/api/v1/me/grants/%d", server, id), nil)
	if err != nil {
		t.Fatal("delete request failed")
	}
	req.Header.Set("Origin", server)
	req.Header.Set("X-CSRF-Token", account.CSRF)
	r, err := c.Do(req)
	if err != nil {
		t.Fatal("delete request failed")
	}
	defer closeBody(r)
	if r.StatusCode != http.StatusNoContent {
		t.Fatalf("delete grant status %d", r.StatusCode)
	}
}

func TestOAuth_MCPFlow(t *testing.T) {
	c := client(t)
	c.Transport = oauthTransport{base: http.DefaultTransport}
	server := base(t, "E2E_SERVER_URL")
	oauthMetadata(t, c, server)
	id := oauthRegister(t, c, server)
	first := oauthAuthorize(t, c, server, id, true)
	oauthMCPWorks(t, c, server, first.Access)
	grantID := oauthGrantID(t, c, server)
	refresh := url.Values{"grant_type": {"refresh_token"}, "client_id": {id}, "resource": {server + "/mcp"}, "refresh_token": {first.Refresh}}
	rotated := oauthToken(t, c, server, refresh)
	if rotated.Access == first.Access || rotated.Refresh == first.Refresh {
		t.Fatal("tokens did not rotate")
	}
	oauthMCPWorks(t, c, server, rotated.Access)
	var replay struct {
		Error string `json:"error"`
	}
	oauthJSON(t, oauthPostForm(t, c, server+"/oauth/token", refresh), http.StatusBadRequest, &replay)
	if replay.Error != "invalid_grant" {
		t.Fatal("refresh replay not rejected")
	}
	for _, grant := range oauthGrants(t, c, server) {
		if grant.ID == grantID {
			t.Fatal("refresh replay did not revoke the grant")
		}
	}
	oauthWaitRevoked(t, c, server, rotated.Access)
	second := oauthAuthorize(t, c, server, id, false)
	oauthMCPWorks(t, c, server, second.Access)
	oauthDeleteGrant(t, c, server)
	oauthWaitRevoked(t, c, server, second.Access)
}
