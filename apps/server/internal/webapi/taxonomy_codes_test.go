package webapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/apierror"
	"danny.vn/trachuan/apps/server/internal/contracttest"
	"danny.vn/trachuan/internal/retrieval"
)

type codesTestEngine struct {
	*testEngine
	codes []string
	err   error
}

func (e *codesTestEngine) TaxonomyCodes(context.Context) ([]string, error) { return e.codes, e.err }

func testPublicTaxonomyCodesContract(t *testing.T, coverage *contracttest.Coverage) {
	h := newHarness(t)
	engine := &codesTestEngine{testEngine: h.engine, codes: []string{"1", "1.1", "2"}}
	h.resolver.engine = engine
	handler := NewPublicTaxonomyCodes(h.webConfig())
	call := func(query string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/public/taxonomy/codes"+query, nil))
		return w
	}
	path := "/api/v1/public/taxonomy/codes"
	w := call("")
	var result struct {
		Release string   `json:"release"`
		Codes   []string `json:"codes"`
	}
	dec := json.NewDecoder(bytes.NewReader(w.Body.Bytes()))
	dec.DisallowUnknownFields()
	if w.Code != 200 || w.Header().Get("Cache-Control") != "public, max-age=600" {
		t.Fatalf("success: %d", w.Code)
	}
	if err := dec.Decode(&result); err != nil || result.Release != "20991110_1" || !reflect.DeepEqual(result.Codes, engine.codes) {
		t.Fatalf("body: %+v %v", result, err)
	}
	coverage.Check(t, "GET", path, w)
	for _, query := range []string{"?x=1", "?node=1", "?", "?x=%zz"} {
		w = call(query)
		if w.Code != 400 {
			t.Fatalf("%s: %d", query, w.Code)
		}
		coverage.Check(t, "GET", path, w)
	}
	engine.err = errors.New("engine failure")
	w = call("")
	if w.Code != 503 {
		t.Fatal(w.Code)
	}
	coverage.Check(t, "GET", path, w)
	engine.err = nil
	for _, codes := range [][]string{nil, {"1", "1"}, {"codes"}} {
		engine.codes = codes
		if w = call(""); w.Code != 503 {
			t.Fatal(w.Code)
		}
	}
	h.resolver.err = retrieval.ErrUnavailable
	if w = call(""); w.Code != 503 {
		t.Fatal(w.Code)
	}
	w = httptest.NewRecorder()
	apierror.RateLimited(w, time.Second)
	coverage.Check(t, "GET", path, w)
}
