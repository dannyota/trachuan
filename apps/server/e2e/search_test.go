//go:build e2e

package e2e

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestSearch_FixtureAndDocument(t *testing.T) {
	c := client(t)
	if loc := login(t, c, "search@example.invalid", true); loc != "/account" {
		t.Fatalf("login destination %q", loc)
	}
	server := base(t, "E2E_SERVER_URL")
	response := get(t, c, server+"/api/v1/me")
	var account struct {
		CSRF string `json:"csrf_token"`
	}
	if response.StatusCode != http.StatusOK {
		closeBody(response)
		t.Fatalf("me status %d", response.StatusCode)
	}
	if err := json.NewDecoder(response.Body).Decode(&account); err != nil {
		closeBody(response)
		t.Fatal(err)
	}
	closeBody(response)
	req, err := http.NewRequest(http.MethodPost, server+"/api/v1/search", strings.NewReader(`{"country":"vn","query":"Văn bản thử nghiệm\nMẫu giả lập vườn thử nghiệm xanh"}`))
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
	var result struct {
		Hits []struct {
			Handle string `json:"handle"`
			Doc    string `json:"doc"`
		} `json:"hits"`
	}
	if response.StatusCode != http.StatusOK {
		closeBody(response)
		t.Fatalf("search status %d", response.StatusCode)
	}
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		closeBody(response)
		t.Fatal(err)
	}
	closeBody(response)
	if len(result.Hits) == 0 || result.Hits[0].Doc != "vn:fixture:1" || result.Hits[0].Handle != "vn:fixture:1#article-1" {
		t.Fatalf("unexpected first hit: %+v", result.Hits)
	}
	response = get(t, c, server+"/api/v1/documents/"+url.PathEscape("vn:fixture:1"))
	defer closeBody(response)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("document status %d", response.StatusCode)
	}
	var document struct {
		Release string          `json:"release"`
		Item    json.RawMessage `json:"item"`
	}
	if err := json.NewDecoder(response.Body).Decode(&document); err != nil {
		t.Fatal(err)
	}
	if document.Release == "" || !strings.Contains(string(document.Item), "Mẫu giả lập vườn thử nghiệm xanh") {
		t.Fatal("fixture document missing")
	}
}
