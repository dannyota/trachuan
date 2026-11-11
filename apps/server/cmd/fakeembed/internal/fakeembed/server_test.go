package fakeembed

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"danny.vn/trachuan/internal/embedding"
)

func TestServer_Contract(t *testing.T) {
	h := NewHandler()
	for _, tc := range []struct {
		path, body, contentType string
		status                  int
	}{
		{"/v1/embed", `{"recipe_hash":"` + embedding.TestRecipeHash + `","texts":["thử nghiệm"]}`, "application/json", http.StatusOK},
		{"/v1/embed", `{"recipe_hash":"wrong","texts":["text"]}`, "application/json", http.StatusConflict},
		{"/v1/embed", `{"recipe_hash":"wrong","recipe_hash":"wrong","texts":["text"]}`, "application/json", http.StatusBadRequest},
		{"/v1/tokenize", `{"recipe_hash":"` + embedding.TestRecipeHash + `","texts":[" "]}`, "application/json", http.StatusBadRequest},
		{"/v1/embed", `{"recipe_hash":"x","texts":["text"]}`, "text/plain", http.StatusUnsupportedMediaType},
	} {
		r := httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(tc.body))
		r.Header.Set("Content-Type", tc.contentType)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Errorf("%s %s: got %d want %d", tc.path, tc.body, w.Code, tc.status)
		}
	}
}

func TestServer_TokenLimitAndBodyLimit(t *testing.T) {
	h := NewHandler()
	for _, tc := range []struct {
		path, text string
		status     int
	}{
		{"/v1/embed", strings.Repeat("x", 254), http.StatusOK},
		{"/v1/embed", strings.Repeat("x", 255), http.StatusUnprocessableEntity},
		{"/v1/tokenize", strings.Repeat("x", 255), http.StatusOK},
		{"/v1/embed", strings.Repeat("x", 65536), http.StatusRequestEntityTooLarge},
	} {
		body := `{"recipe_hash":"` + embedding.TestRecipeHash + `","texts":["` + tc.text + `"]}`
		r := httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Errorf("%s text length %d: got %d want %d", tc.path, len(tc.text), w.Code, tc.status)
		}
	}
}

func TestServer_EmbeddingMatchesFixtureVector(t *testing.T) {
	h := NewHandler()
	text := "Mẫu giả lập"
	body := `{"recipe_hash":"` + embedding.TestRecipeHash + `","texts":["` + text + `"]}`
	r := httptest.NewRequest(http.MethodPost, "/v1/embed", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	var response struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	want := embedding.FakeVector(text)
	if len(response.Data) != 1 || !slices.Equal(response.Data[0].Embedding, want) {
		t.Fatal("returned vector differs from fixture vector")
	}
}
