package webapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/limits"
	"danny.vn/trachuan/apps/server/internal/mcpapi"
	"danny.vn/trachuan/apps/server/internal/websession"
	"danny.vn/trachuan/internal/frameworks/serve"
	"github.com/stretchr/testify/require"
)

type frameworkEngine struct {
	read serve.ReadRequest
	err  error
}

func (*frameworkEngine) FrameworkCodes() []string { return []string{"fw-alpha"} }
func (e *frameworkEngine) Search(context.Context, serve.Request, serve.Caller) (serve.Response, error) {
	return serve.Response{Release: "20990101", AccessSetVersion: "invented-access", Hits: []serve.Hit{{Handle: "fw:fw-alpha:1#A", Citation: "A", Kind: "control", TextWithheld: true}}}, e.err
}
func (e *frameworkEngine) Read(_ context.Context, r serve.ReadRequest, _ serve.Caller) (serve.ReadResponse, error) {
	e.read = r
	return serve.ReadResponse{Metadata: frameworkMetadata(), Items: []serve.ReadItemResult{{Handle: "fw:fw-alpha:1#A", Complete: true, Units: []serve.ReadUnit{{Kind: "head", Head: &serve.ReadHead{Handle: "fw:fw-alpha:1#A", Citation: "A", Kind: "control", Access: "licensed", TextWithheld: true}}}}}}, e.err
}
func (e *frameworkEngine) CorpusStatus(context.Context, serve.StatusRequest, serve.Caller) (serve.StatusResponse, error) {
	return serve.StatusResponse{Metadata: frameworkMetadata(), RetrievalRecipeStatus: "placeholder", Versions: map[string]serve.VersionStatus{"fw:fw-alpha:1": {Access: "licensed"}}}, e.err
}
func (e *frameworkEngine) QualityGaps(context.Context, serve.GapsRequest, serve.Caller) (serve.GapsResponse, error) {
	return serve.GapsResponse{Metadata: frameworkMetadata(), RetrievalRecipeStatus: "placeholder", Gaps: []serve.Gap{{Kind: "title_missing", Version: "fw:fw-alpha:1", Detail: "1"}}}, e.err
}
func (e *frameworkEngine) Mappings(context.Context, serve.MappingsRequest, serve.Caller) (serve.MappingsResponse, error) {
	end := serve.MappingEnd{Handle: "fw:fw-alpha:1#A", Citation: "A", Version: "1", Title: "Invented title"}
	return serve.MappingsResponse{Metadata: frameworkMetadata(), Edges: []serve.MappingEdge{{From: end, To: end, Relation: "related", Source: "invented", Resolved: true}}}, e.err
}

func frameworkMetadata() serve.Metadata {
	return serve.Metadata{Release: "20990101", AccessSetVersion: "invented-access"}
}

type frameworkResolver struct {
	engine              *frameworkEngine
	calls, done, access int
	err                 error
}

func (r *frameworkResolver) ResolveFrameworks(context.Context, string) (mcpapi.FrameworkLease, error) {
	r.calls++
	return mcpapi.FrameworkLease{Engine: r.engine, Done: func() { r.done++ }}, r.err
}

func frameworkHarness(enabled bool) (http.Handler, *frameworkResolver) {
	now := time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	r := &frameworkResolver{engine: &frameworkEngine{}}
	cfg := mcpapi.Config{Frameworks: enabled, FrameworkResolver: r, Limiter: limits.New(clock), Clock: clock,
		FrameworkAccess: func(context.Context, int64, []string) (serve.Caller, error) {
			r.access++
			return serve.Caller{}, nil
		}}
	return New(Config{Tools: cfg, Sessions: &websession.Manager{Store: sessionStore{now: now}, Clock: clock, PublicURL: "https://example.test"}}), r
}

type frameworkRoute struct{ method, path, actual, body string }

func frameworkRoutes() []frameworkRoute {
	return []frameworkRoute{
		{"GET", "/api/v1/frameworks", "/api/v1/frameworks", ""},
		{"GET", "/api/v1/frameworks/versions/{handle}", "/api/v1/frameworks/versions/" + url.PathEscape("fw:fw-alpha:1"), ""},
		{"GET", "/api/v1/frameworks/controls/{handle}", "/api/v1/frameworks/controls/" + url.PathEscape("fw:fw-alpha:1#A"), ""},
		{"POST", "/api/v1/frameworks/search", "/api/v1/frameworks/search", `{"corpus":"frameworks","query":"invented"}`},
		{"GET", "/api/v1/frameworks/mappings", "/api/v1/frameworks/mappings?handle=" + url.QueryEscape("fw:fw-alpha:1#A"), ""},
		{"GET", "/api/v1/frameworks/gaps", "/api/v1/frameworks/gaps", ""},
	}
}

func TestFrameworks_RoutesAndGuards(t *testing.T) {
	for _, route := range frameworkRoutes() {
		for _, guard := range []string{"success", "off", "anonymous", "bearer", "csrf"} {
			if guard == "csrf" && route.method != "POST" {
				continue
			}
			t.Run(route.path+"/"+guard, func(t *testing.T) {
				h, resolver := frameworkHarness(guard != "off")
				r := webRequest(route.method, route.actual, route.body)
				want := 200
				switch guard {
				case "off":
					want = 404
					r.Header.Del("Cookie")
				case "anonymous", "bearer":
					want = 401
					r.Header.Del("Cookie")
					if guard == "bearer" {
						r.Header.Set("Authorization", "Bearer trc_"+strings.Repeat("A", 43))
					}
				case "csrf":
					want = 403
					r.Header.Del("X-CSRF-Token")
				}
				w := httptest.NewRecorder()
				h.ServeHTTP(w, r)
				require.Equal(t, want, w.Code)
				require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
				if want != 200 {
					require.Zero(t, resolver.calls)
					require.Zero(t, resolver.access)
				} else {
					require.Equal(t, 1, resolver.calls)
					require.Equal(t, 1, resolver.access)
					require.Equal(t, 1, resolver.done)
				}
			})
		}
	}
}

func TestFrameworks_HandleCanonicalityBeforeLookup(t *testing.T) {
	for _, tc := range []struct{ kind, handle string }{
		{"versions", "fw:fw-alpha:1#A"}, {"controls", "fw:fw-alpha:1"},
		{"controls", "fw:fw-alpha:1#%41"}, {"controls", "fw:fw-alpha:1#A%2fb"},
		{"controls", url.PathEscape("fw:fw-alpha:1#A")}, {"versions", "fw%3Afw-alpha%3A1"},
	} {
		t.Run(tc.kind+tc.handle, func(t *testing.T) {
			h, resolver := frameworkHarness(true)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, webRequest("GET", "/api/v1/frameworks/"+tc.kind+"/"+url.PathEscape(tc.handle), ""))
			require.Equal(t, 400, w.Code)
			require.JSONEq(t, `{"error":{"code":"invalid_handle","message":"Invalid handle."}}`, w.Body.String())
			require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
			require.Zero(t, resolver.calls)
			require.Zero(t, resolver.access)
		})
	}
}

func TestFrameworks_EncodedCitationAndContinuation(t *testing.T) {
	for _, handle := range []string{"fw:fw-alpha:1#A%2FB", "fw:fw-alpha:1#A%20B", "fw:fw-alpha:1#A%25B"} {
		for _, next := range []string{"", "invented-next"} {
			h, resolver := frameworkHarness(true)
			path := "/api/v1/frameworks/controls/" + url.PathEscape(handle)
			if next != "" {
				path += "?next=" + next
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, webRequest("GET", path, ""))
			require.Equal(t, 200, w.Code)
			require.Equal(t, handle, resolver.engine.read.ExpectedHandle)
			want := serve.ReadItem{Handle: handle}
			if next != "" {
				want = serve.ReadItem{Next: next}
			}
			require.Equal(t, []serve.ReadItem{want}, resolver.engine.read.Items)
		}
	}
}

func TestFrameworks_AuthStoreUnavailable(t *testing.T) {
	h := New(Config{Tools: mcpapi.Config{Frameworks: true}, Sessions: &websession.Manager{}})
	for _, route := range frameworkRoutes() {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, webRequest(route.method, route.actual, route.body))
		require.Equal(t, 503, w.Code)
		require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
		require.JSONEq(t, `{"error":{"code":"unavailable","message":"Service unavailable."}}`, w.Body.String())
	}
}
