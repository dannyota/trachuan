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
			Handle   string `json:"handle"`
			Doc      string `json:"doc"`
			Warnings []struct {
				Kind     string `json:"kind"`
				Key      string `json:"key"`
				Number   string `json:"number"`
				Source   string `json:"source"`
				Basis    string `json:"basis"`
				Exact    bool   `json:"exact"`
				Named    string `json:"named"`
				Citation string `json:"citation"`
			} `json:"warnings"`
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
	warnings := result.Hits[0].Warnings
	if len(warnings) != 3 || warnings[0].Kind != "amends_supplements" || !warnings[0].Exact || warnings[0].Citation != "Điều 1" || warnings[1].Named != "article-1/khoan-9" || warnings[1].Exact || warnings[2].Basis != "vbpl_reference" || !warnings[2].Exact {
		t.Fatalf("search warnings: %+v", warnings)
	}
	for _, warning := range warnings {
		if warning.Key != "vn:fixture:warnings" || warning.Number != "28/2099/TT-TEST" || warning.Source != "vn:fixture:warnings#article-1" {
			t.Fatalf("warning source: %+v", warning)
		}
	}
	readReq, err := http.NewRequest(http.MethodPost, server+"/api/v1/read", strings.NewReader(`{"country":"vn","items":[{"handle":"vn:fixture:1#article-1","relations":true}]}`))
	if err != nil {
		t.Fatal(err)
	}
	readReq.Header.Set("Content-Type", "application/json")
	readReq.Header.Set("Origin", server)
	readReq.Header.Set("X-CSRF-Token", account.CSRF)
	response, err = c.Do(readReq)
	if err != nil {
		t.Fatal(err)
	}
	var read struct {
		Items []struct {
			Units []struct {
				Relation struct {
					Evidence string `json:"evidence"`
				} `json:"relation"`
			} `json:"units"`
		} `json:"items"`
	}
	if response.StatusCode != http.StatusOK {
		closeBody(response)
		t.Fatalf("read status %d", response.StatusCode)
	}
	if err = json.NewDecoder(response.Body).Decode(&read); err != nil {
		closeBody(response)
		t.Fatal(err)
	}
	closeBody(response)
	if len(read.Items) != 1 {
		t.Fatalf("read: %+v", read)
	}
	var evidence string
	for _, unit := range read.Items[0].Units {
		evidence += unit.Relation.Evidence
	}
	for _, expected := range []string{"exact evidence", "inexact evidence", "reference evidence", "diagram evidence"} {
		if !strings.Contains(evidence, expected) {
			t.Fatalf("missing %q: %s", expected, evidence)
		}
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
