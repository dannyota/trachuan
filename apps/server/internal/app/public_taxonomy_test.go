package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/credential"
	"danny.vn/trachuan/apps/server/internal/mcpapi"
	"danny.vn/trachuan/apps/server/internal/weblogin"
	"danny.vn/trachuan/apps/server/internal/websession"
	"danny.vn/trachuan/internal/retrieval"
	"golang.org/x/oauth2"
)

type publicTaxonomyEngine struct{ drainEngine }

func (*publicTaxonomyEngine) Status(context.Context) (retrieval.CorpusStatus, error) {
	return retrieval.CorpusStatus{Release: "20991110_1"}, nil
}
func (*publicTaxonomyEngine) Taxonomy(context.Context, retrieval.TaxonomyRequest) (retrieval.TaxonomyResult, error) {
	return retrieval.TaxonomyResult{Release: "20991110_1", Nodes: []retrieval.NodeEntry{{Code: "1", Name: "Root", Level: 1}}}, nil
}
func (*publicTaxonomyEngine) TaxonomyNode(context.Context, string) (retrieval.NodePage, error) {
	return retrieval.NodePage{Release: "20991110_1", Node: retrieval.NodeEntry{Code: "1", Name: "Root", Level: 1}}, nil
}

type publicTaxonomyResolver struct{ engine *publicTaxonomyEngine }

func (r publicTaxonomyResolver) Resolve(context.Context, string, string) (mcpapi.Lease, error) {
	return mcpapi.Lease{Engine: r.engine, Release: "20991110_1", Done: func() {}}, nil
}

func TestPublicTaxonomy_MountedSharedIPLimit(t *testing.T) {
	now := time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	sessions := &websession.Manager{Store: routeSessionStore{}, PublicURL: "http://localhost"}
	ctx := context.WithValue(context.Background(), oauth2.HTTPClient, &http.Client{Transport: discoveryRoundTrip{}})
	login, err := weblogin.New(ctx, weblogin.Config{ClientID: "id", ClientSecret: "configured", PublicURL: "http://localhost", Issuer: "https://issuer.test", Store: &routeLoginStore{}, Sessions: sessions})
	if err != nil {
		t.Fatal(err)
	}
	limiter := &routeLimiter{}
	verifier := &credential.Verifier{Store: drainStore{}, Audience: "http://localhost/mcp"}
	mux := newWebMux(webDeps{store: routeAccountStore{}, sessions: sessions, login: login, verifier: verifier, tools: mcpapi.Config{Limiter: limiter, Resolver: publicTaxonomyResolver{engine: &publicTaxonomyEngine{}}, Clock: clock}, loginLimit: newLoginLimiter(clock, nil)})
	paths := []string{"/api/v1/public/status", "/api/v1/public/taxonomy", "/api/v1/public/taxonomy/1"}
	call := func(path, ip string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", path, nil)
		r.RemoteAddr = ip + ":1234"
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w
	}
	for i := 0; i < 60; i++ {
		path := paths[i%len(paths)]
		w := call(path, "192.0.2.1")
		if w.Code != 200 || w.Header().Get("Cache-Control") != "public, max-age=600" {
			t.Fatalf("%s request %d: %d %s", path, i, w.Code, w.Body.String())
		}
	}
	for _, path := range paths {
		w := call(path, "192.0.2.1")
		if w.Code != 429 || w.Header().Get("Retry-After") == "" || !strings.Contains(w.Body.String(), `"code":"rate_limited"`) {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body.String())
		}
		w = call(path, "192.0.2.2")
		if w.Code != 200 || w.Header().Get("Cache-Control") != "public, max-age=600" {
			t.Fatalf("other IP %s: %d %s", path, w.Code, w.Body.String())
		}
	}
	if len(limiter.calls) != 0 {
		t.Fatal("public routes used account limiter")
	}
}
