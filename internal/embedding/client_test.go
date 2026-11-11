package embedding

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClient_DecodesErrorTable(t *testing.T) {
	for _, status := range []int{400, 409, 413, 415, 422, 429, 500, 503, 504} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
				_, _ = fmt.Fprint(w, `{"error":{"code":"failure","message":"fixed","index":1,"token_count":42}}`)
			})
			_, err := NewClient("http://embedder", fakeClient(handler)).Tokenize(context.Background(), "hash", []string{"text"})
			var e *Error
			if !errors.As(err, &e) || e.Status != status || e.Code != "failure" || e.Index == nil || *e.Index != 1 || e.TokenCount == nil || *e.TokenCount != 42 {
				t.Fatalf("unexpected error: %#v", err)
			}
		})
	}
}

func TestClient_RejectsWrongDimensionsOrIndex(t *testing.T) {
	for _, body := range []string{`{"recipe_hash":"h","dimensions":2,"data":[{"index":0,"token_count":2,"embedding":[1]}]}`, `{"recipe_hash":"h","dimensions":1,"data":[{"index":1,"token_count":2,"embedding":[1]}]}`} {
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = fmt.Fprint(w, body) })
		_, err := NewClient("http://embedder", fakeClient(handler)).Embed(context.Background(), "h", []string{"text"})
		if err == nil {
			t.Fatalf("accepted %s", body)
		}
	}
}

type handlerTransport struct{ handler http.Handler }

func (h handlerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	w := httptest.NewRecorder()
	h.handler.ServeHTTP(w, r)
	return w.Result(), nil
}
func fakeClient(h http.Handler) *http.Client { return &http.Client{Transport: handlerTransport{h}} }

func TestQuery_RejectsChangedRecipe(t *testing.T) {
	r := QueryRecipe1()
	client := NewClient("http://embedder", fakeClient(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(Report{ProtocolVersion: 1, RecipeHash: r.Hash(), Recipe: r})
	})))
	_, err := (Query{Client: client, RecipeHash: "wrong"}).Recipe(context.Background())
	if err == nil {
		t.Fatal("accepted changed recipe")
	}
}

func TestQuery_ReportsRecipeDifferences(t *testing.T) {
	expected := QueryRecipe1()
	running := expected
	running.Language = "go 1.27.4\nextra"
	running.TokenCap = 128
	client := NewClient("http://embedder", fakeClient(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(Report{ProtocolVersion: 1, RecipeHash: running.Hash(), Recipe: running})
	})))
	_, err := (Query{Client: client, RecipeHash: expected.Hash(), ExpectedRecipe: &expected}).Recipe(context.Background())
	for _, want := range []string{`language: expected "go 1.27.3", running "go 1.27.4\nextra"`, `token_cap: expected 256, running 128`} {
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("error = %v, missing %s", err, want)
		}
	}
	if err != nil && strings.Contains(err.Error(), "\n") {
		t.Errorf("error spans lines: %q", err)
	}
}

func TestQuery_ReportsHashesWithoutExpectedRecipe(t *testing.T) {
	running := QueryRecipe1()
	client := NewClient("http://embedder", fakeClient(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(Report{ProtocolVersion: 1, RecipeHash: running.Hash(), Recipe: running})
	})))
	_, err := (Query{Client: client, RecipeHash: "expected-hash"}).Recipe(context.Background())
	if err == nil || !strings.Contains(err.Error(), "expected-hash") || !strings.Contains(err.Error(), running.Hash()) {
		t.Fatalf("missing hashes: %v", err)
	}
}

func TestQuery_RejectsInconsistentExpectedRecipe(t *testing.T) {
	expected := QueryRecipe1()
	_, err := (Query{RecipeHash: "wrong", ExpectedRecipe: &expected}).Recipe(context.Background())
	if err == nil || !strings.Contains(err.Error(), "query recipe mismatch") {
		t.Fatalf("inconsistent expected recipe: %v", err)
	}
}
