package webapi

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/limits"
	"danny.vn/trachuan/apps/server/internal/mcpapi"
	"danny.vn/trachuan/apps/server/internal/websession"
	"danny.vn/trachuan/internal/retrieval"
)

func TestWeb_RejectsInvalidToolInputs(t *testing.T) {
	for _, tc := range []struct{ path, body string }{
		{"search", `{"country":"us","query":"ngân hàng"}`},
		{"search", `{"country":"vn","query":"ngân hàng","limit":0}`},
		{"search", `{"country":"vn","query":"ngân hàng","detail":"other"}`},
		{"search", `{"country":"vn","query":"ngân hàng","as_of":"2099-02-30"}`},
		{"search", `{"country":"vn","query":null}`},
		{"search", `{"country":"vn","query":"ngân hàng","extra":true}`},
		{"search", `{"country":"vn","query":"ngân hàng"} {}`},
		{"search", `[]`},
		{"read", `{"country":"vn","items":null}`},
		{"read", `{"country":"vn","items":[{"handle":"vn:1","extra":true}]}`},
		{"read", `{"country":"vn","items":[],"page_cap":2000000}`},
		{"read", `{"country":"vn","items":[{},{},{},{},{},{},{},{},{}]}`},
	} {
		t.Run(tc.path+tc.body, func(t *testing.T) {
			h := newHarness(t)
			w := webCall(h, "POST", "/api/v1/"+tc.path, tc.body)
			if w.Code != 400 || !strings.Contains(w.Body.String(), `"code":"invalid_request"`) {
				t.Fatal(w.Code, w.Body.String())
			}
			if h.resolver.done.Load() != 0 {
				t.Fatal("invalid input reached resolver")
			}
		})
	}
}

func TestWeb_BodyCapIncludesChunkedBodies(t *testing.T) {
	for _, chunked := range []bool{false, true} {
		h := newHarness(t)
		r := webRequest("POST", "/api/v1/search", `{"country":"vn","query":"ngân hàng"}`+strings.Repeat(" ", 64<<10))
		if chunked {
			r.ContentLength = -1
			r.TransferEncoding = []string{"chunked"}
		}
		w := httptest.NewRecorder()
		h.web.ServeHTTP(w, r)
		if w.Code != 413 || h.resolver.done.Load() != 0 {
			t.Fatal("body cap failed", w.Code)
		}
	}
}

func TestWeb_SessionAndCSRFRequired(t *testing.T) {
	for _, change := range []func(*http.Request){
		func(r *http.Request) {
			r.Header.Del("Cookie")
			r.Header.Set("Authorization", "Bearer trc_"+strings.Repeat("A", 43))
		},
		func(r *http.Request) { r.Header.Del("X-CSRF-Token") },
		func(r *http.Request) { r.Header.Set("Origin", "https://attacker.test") },
		func(r *http.Request) { r.Header.Set("Content-Type", "text/plain") },
	} {
		h := newHarness(t)
		r := webRequest("POST", "/api/v1/search", `{"country":"vn","query":"ngân hàng"}`)
		change(r)
		w := httptest.NewRecorder()
		h.web.ServeHTTP(w, r)
		if (w.Code != 401 && w.Code != 403) || h.resolver.done.Load() != 0 {
			t.Fatal("guard failed", w.Code)
		}
	}
}

func TestDocuments_QueryValidation(t *testing.T) {
	for _, path := range []string{"/api/v1/documents/us:1", "/api/v1/documents/vn:1%23a", "/api/v1/documents/vn:1?release=a&release=b", "/api/v1/documents/vn:1?next=", "/api/v1/documents/vn:1?other=x", "/api/v1/documents/vn:1?next=%zz", "/api/v1/status?other=x"} {
		h := newHarness(t)
		w := webCall(h, "GET", path, "")
		if w.Code != 400 || h.resolver.done.Load() != 0 {
			t.Fatal("invalid document URL reached resolver", path, w.Code)
		}
	}
}

func TestWeb_ErrorsAreFixedAndLogsExcludeInput(t *testing.T) {
	for _, tc := range []struct {
		err    error
		status int
		code   string
	}{{retrieval.ErrEmbedderBusy, 503, "busy"}, {retrieval.ErrEmbedder, 503, "unavailable"}, {retrieval.ErrReleaseGone, 409, "release_expired"}, {errors.New("failure"), 503, "unavailable"}} {
		now := time.Date(2099, 11, 10, 0, 0, 0, 0, time.UTC)
		clock := func() time.Time { return now }
		logs := &bytes.Buffer{}
		e := &testEngine{err: fmt.Errorf("sensitive-fixture: %w", tc.err)}
		resolver := &testResolver{engine: e}
		cfg := Config{Tools: mcpapi.Config{Resolver: resolver, Limiter: limits.New(clock), Clock: clock, Logger: slog.New(slog.NewTextHandler(logs, nil))}, Sessions: &websession.Manager{Store: sessionStore{now: now}, Clock: clock, PublicURL: "https://example.test"}}
		h := &harness{web: New(cfg)}
		w := webCall(h, "POST", "/api/v1/search", `{"country":"vn","query":"ngân hàng sensitive-fixture"}`)
		if w.Code != tc.status || !strings.Contains(w.Body.String(), `"code":"`+tc.code+`"`) {
			t.Fatal(w.Code, w.Body.String())
		}
		if strings.Contains(w.Body.String(), "sensitive-fixture") || strings.Contains(logs.String(), "sensitive-fixture") || strings.Contains(logs.String(), strings.Repeat("A", 43)) {
			t.Fatal("sensitive input leaked")
		}
		if resolver.done.Load() != 1 {
			t.Fatal("failure leaked lease")
		}
	}
}
