package webapi

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/apierror"
	"danny.vn/trachuan/apps/server/internal/contracttest"
	"danny.vn/trachuan/apps/server/internal/release"
	"danny.vn/trachuan/internal/retrieval"
)

func TestPublicTaxonomy_RejectsLookupText(t *testing.T) {
	h := newHarness(t)
	w := httptest.NewRecorder()
	NewPublicTaxonomy(h.webConfig()).ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/public/taxonomy?q=private", nil))
	if w.Code != 400 {
		t.Fatalf("status %d", w.Code)
	}
}

func TestPublicTaxonomy_OpenAPIPaths(t *testing.T) {
	c, err := contracttest.Load()
	if err != nil {
		t.Fatal(err)
	}
	coverage := contracttest.NewCoverageForTag(c, "web")
	h := newHarness(t)
	handler := NewPublicTaxonomy(h.webConfig())
	for _, path := range []string{"/api/v1/public/taxonomy", "/api/v1/public/taxonomy/{code}"} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/api/v1/public/taxonomy?q=x", nil)
		handler.ServeHTTP(w, r)
		coverage.Check(t, "GET", path, w)
	}
}

func (e *testEngine) Taxonomy(_ context.Context, req retrieval.TaxonomyRequest) (retrieval.TaxonomyResult, error) {
	if e.err != nil {
		return retrieval.TaxonomyResult{}, e.err
	}
	if req.Node == "999" {
		return retrieval.TaxonomyResult{}, retrieval.ErrNotFound
	}
	return retrieval.TaxonomyResult{Release: "20991110_1", Nodes: []retrieval.NodeEntry{{Code: "1", Name: "Name", Level: 1, Origin: "phapdien", Provisions: 2, Documents: 1}}}, nil
}
func (e *testEngine) TaxonomyNode(_ context.Context, code string) (retrieval.NodePage, error) {
	if e.err != nil {
		return retrieval.NodePage{}, e.err
	}
	if code == "bad" {
		return retrieval.NodePage{}, retrieval.ErrInvalidRequest
	}
	if code == "999" {
		return retrieval.NodePage{}, retrieval.ErrNotFound
	}
	return retrieval.NodePage{Release: "20991110_1", Node: retrieval.NodeEntry{Code: "1", Name: "Name", Level: 1}}, nil
}

func testPublicTaxonomyContract(t *testing.T, coverage *contracttest.Coverage) {
	for _, tc := range []struct{ path, code string }{{"/api/v1/public/taxonomy", ""}, {"/api/v1/public/taxonomy/{code}", "1"}} {
		h := newHarness(t)
		handler := NewPublicTaxonomy(h.webConfig())
		call := func(query, code string) *httptest.ResponseRecorder {
			r := httptest.NewRequest("GET", "/api/v1/public/taxonomy"+query, nil)
			r.SetPathValue("code", code)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			return w
		}
		w := call("", tc.code)
		if w.Code != 200 || w.Header().Get("Cache-Control") != "public, max-age=600" {
			t.Fatalf("success: %d %s", w.Code, w.Body.String())
		}
		coverage.Check(t, "GET", tc.path, w)
		w = call("?q=private", tc.code)
		if w.Code != 400 {
			t.Fatal(w.Code)
		}
		coverage.Check(t, "GET", tc.path, w)
		unknown := "999"
		query := ""
		if tc.code == "" {
			query = "?node=999"
			unknown = ""
		}
		w = call(query, unknown)
		if w.Code != 404 {
			t.Fatal(w.Code, w.Body.String())
		}
		coverage.Check(t, "GET", tc.path, w)
		h.resolver.err = release.ErrExpired
		w = call("", tc.code)
		if w.Code != 409 {
			t.Fatal(w.Code)
		}
		coverage.Check(t, "GET", tc.path, w)
		h.resolver.err = retrieval.ErrUnavailable
		w = call("", tc.code)
		if w.Code != 503 {
			t.Fatal(w.Code)
		}
		coverage.Check(t, "GET", tc.path, w)
		w = httptest.NewRecorder()
		apierror.RateLimited(w, time.Second)
		coverage.Check(t, "GET", tc.path, w)
	}
	h := newHarness(t)
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/v1/public/taxonomy/bad", nil)
	r.SetPathValue("code", "bad")
	NewPublicTaxonomy(h.webConfig()).ServeHTTP(w, r)
	if w.Code != 400 {
		t.Fatal(w.Code)
	}
}
