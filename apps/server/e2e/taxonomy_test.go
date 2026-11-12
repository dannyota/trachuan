//go:build e2e

package e2e

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"golang.org/x/oauth2"
)

func TestTaxonomy_Flow(t *testing.T) {
	c := client(t)
	login(t, c, "taxonomy@example.invalid", true)
	server := base(t, "E2E_SERVER_URL")
	response := get(t, c, server+"/api/v1/public/taxonomy/90.1")
	if response.StatusCode != http.StatusOK || response.Header.Get("Cache-Control") != "public, max-age=600" {
		t.Fatalf("taxonomy status %d cache %q", response.StatusCode, response.Header.Get("Cache-Control"))
	}
	closeBody(response)
	token := taxonomyToken(t, c, server)
	req, err := http.NewRequest(http.MethodPost, server+"/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"taxonomy","arguments":{"country":"vn"}}}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	response, err = c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var rpc struct {
		Result struct {
			Structured struct {
				Nodes []struct {
					Code       string `json:"code"`
					Provisions int    `json:"provisions"`
					Documents  int    `json:"documents"`
				} `json:"nodes"`
			} `json:"structuredContent"`
			IsError bool `json:"isError"`
		} `json:"result"`
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("MCP status %d", response.StatusCode)
	}
	if err := json.NewDecoder(response.Body).Decode(&rpc); err != nil {
		t.Fatal(err)
	}
	closeBody(response)
	if rpc.Result.IsError || len(rpc.Result.Structured.Nodes) != 1 || rpc.Result.Structured.Nodes[0].Code != "90" || rpc.Result.Structured.Nodes[0].Provisions != 1 || rpc.Result.Structured.Nodes[0].Documents != 1 {
		t.Fatalf("MCP taxonomy: %+v", rpc.Result)
	}
	response = get(t, c, server+"/api/v1/me")
	var account struct {
		CSRF string `json:"csrf_token"`
	}
	if err := json.NewDecoder(response.Body).Decode(&account); err != nil {
		t.Fatal(err)
	}
	closeBody(response)
	input := map[string]any{"country": "vn", "query": "Văn bản thử nghiệm\nMẫu phân loại giả lập số 1.", "nodes": []string{"90.1"}, "limit": 20}
	var previous string
	for page := 0; page < 2; page++ {
		body, err := json.Marshal(input)
		if err != nil {
			t.Fatal(err)
		}
		req, err := http.NewRequest(http.MethodPost, server+"/api/v1/search", bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", server)
		req.Header.Set("X-CSRF-Token", account.CSRF)
		response, err = c.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		if response.StatusCode != http.StatusOK {
			t.Fatalf("search page %d status %d", page, response.StatusCode)
		}
		var result struct {
			Hits []struct {
				Handle   string `json:"handle"`
				InFilter *bool  `json:"in_filter"`
			} `json:"hits"`
			Outside *int   `json:"outside_filter_count"`
			Cursor  string `json:"next_cursor"`
			Release string `json:"release"`
		}
		if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
			t.Fatal(err)
		}
		closeBody(response)
		if len(result.Hits) == 0 || result.Outside == nil || *result.Outside == 0 {
			t.Fatalf("search page %d: %+v", page, result)
		}
		for _, hit := range result.Hits {
			if hit.InFilter == nil || hit.Handle == previous {
				t.Fatalf("invalid hit: %+v", hit)
			}
			previous = hit.Handle
		}
		if page == 0 {
			if result.Cursor == "" || !*result.Hits[0].InFilter {
				t.Fatal("missing cursor or inside hit")
			}
			input["cursor"] = result.Cursor
			input["release"] = result.Release
		}
	}
}

func taxonomyToken(t *testing.T, c *http.Client, server string) string {
	t.Helper()
	const redirect = "http://127.0.0.1:12345/callback"
	r, err := c.Post(server+"/oauth/register", "application/json", strings.NewReader(`{"client_name":"Fixture","redirect_uris":["http://127.0.0.1:12345/callback"],"grant_types":["authorization_code","refresh_token"],"response_types":["code"],"token_endpoint_auth_method":"none"}`))
	if err != nil {
		t.Fatal(err)
	}
	if r.StatusCode != http.StatusCreated {
		t.Fatalf("registration status %d", r.StatusCode)
	}
	var registration struct {
		Client string `json:"client_id"`
	}
	if decodeErr := json.NewDecoder(r.Body).Decode(&registration); decodeErr != nil {
		t.Fatal(decodeErr)
	}
	closeBody(r)
	verifier := strings.Repeat("a", 43)
	q := url.Values{"client_id": {registration.Client}, "redirect_uri": {redirect}, "response_type": {"code"}, "code_challenge": {oauth2.S256ChallengeFromVerifier(verifier)}, "code_challenge_method": {"S256"}, "state": {"fixture-state"}, "resource": {server + "/mcp"}, "scope": {"mcp:read"}}
	r = get(t, c, server+"/oauth/authorize?"+q.Encode())
	path := r.Header.Get("Location")
	closeBody(r)
	if r.StatusCode != http.StatusSeeOther || !strings.HasPrefix(path, "/oauth/consent/") {
		t.Fatalf("authorization status %d", r.StatusCode)
	}
	form := fields(t, get(t, c, server+path))
	form.Set("decision", "approve")
	req, err := http.NewRequest(http.MethodPost, server+path, strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Origin", server)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r, err = c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	location, err := r.Location()
	closeBody(r)
	if err != nil || r.StatusCode != http.StatusSeeOther || location.Query().Get("code") == "" {
		t.Fatal("consent failed")
	}
	exchange := url.Values{"grant_type": {"authorization_code"}, "client_id": {registration.Client}, "redirect_uri": {redirect}, "code": {location.Query().Get("code")}, "code_verifier": {verifier}, "resource": {server + "/mcp"}}
	r, err = c.PostForm(server+"/oauth/token", exchange)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Access string `json:"access_token"`
	}
	if r.StatusCode != http.StatusOK {
		t.Fatalf("token status %d", r.StatusCode)
	}
	if decodeErr := json.NewDecoder(r.Body).Decode(&result); decodeErr != nil {
		t.Fatal(decodeErr)
	}
	closeBody(r)
	if result.Access == "" {
		t.Fatal("missing token")
	}
	return result.Access
}
