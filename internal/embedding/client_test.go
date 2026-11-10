package embedding

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
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
