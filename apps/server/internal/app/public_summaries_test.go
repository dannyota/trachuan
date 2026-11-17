package app

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/contracttest"
	"danny.vn/trachuan/apps/server/internal/credential"
	"danny.vn/trachuan/apps/server/internal/mcpapi"
	"danny.vn/trachuan/apps/server/internal/publicsummary"
	"danny.vn/trachuan/apps/server/internal/store"
	"danny.vn/trachuan/apps/server/internal/weblogin"
	"danny.vn/trachuan/apps/server/internal/websession"
	"golang.org/x/oauth2"
)

type summaryRoutePool struct{}

func (summaryRoutePool) Close() {}
func (summaryRoutePool) Row(context.Context, string) (publicsummary.Row, error) {
	return publicsummary.Row{Key: "vn:vbpl:1", StatusClass: "unknown", StatusBasis: "source_status", Payload: []byte(`{"number":"1","title":"Title","type":"law","issuer":"Issuer","issued":null,"has_text":true,"text_complete":true,"source_url":null,"relations":[],"relations_total":0,"nodes":[],"outline":[]}`)}, nil
}
func (summaryRoutePool) List(context.Context) ([]publicsummary.Item, error) {
	return []publicsummary.Item{{Key: "vn:vbpl:1", Index: false}}, nil
}

type summaryCredentialStore struct {
	websession.Store
	now   time.Time
	reads *atomic.Int64
}

func (s summaryCredentialStore) GetSession(_ context.Context, hash string) (store.GetSessionRow, error) {
	s.reads.Add(1)
	return store.GetSessionRow{ID: 1, AccountID: 1, TokenHash: hash, CsrfSecret: strings.Repeat("B", 43), CreatedAt: s.now, LastSeenAt: s.now, ExpiresAt: s.now.Add(time.Hour)}, nil
}
func (s summaryCredentialStore) GetCredential(context.Context, string) (store.GetCredentialRow, error) {
	s.reads.Add(1)
	return store.GetCredentialRow{AccountID: 1, ExpiresAt: s.now.Add(time.Hour)}, nil
}
func (summaryCredentialStore) TouchPersonalToken(context.Context, string) error { return nil }

type summaryReaderResolver struct{ calls *atomic.Int64 }

func (r summaryReaderResolver) Resolve(context.Context, string, string) (mcpapi.Lease, error) {
	r.calls.Add(1)
	return mcpapi.Lease{}, publicsummary.ErrUnavailable
}

func TestPublicSummaries_MountedCredentialEquivalenceWithoutAuthOrAddressLimits(t *testing.T) {
	now := time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	reads, readerCalls := &atomic.Int64{}, &atomic.Int64{}
	auth := summaryCredentialStore{now: now, reads: reads}
	session := func() *websession.Manager {
		return &websession.Manager{Store: auth, Clock: clock, PublicURL: "http://localhost"}
	}
	verifier := func() *credential.Verifier {
		return &credential.Verifier{Store: auth, Clock: clock, Audience: "http://localhost/mcp"}
	}
	valid := httptest.NewRequest("GET", "/", nil)
	valid.AddCookie(&http.Cookie{Name: "__Host-session", Value: strings.Repeat("A", 43)})
	session().Middleware(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		if _, ok := websession.FromContext(r.Context()); !ok {
			t.Fatal("fixture session invalid")
		}
	})).ServeHTTP(httptest.NewRecorder(), valid)
	if _, err := verifier().Verify(context.Background(), "trc_"+strings.Repeat("A", 43)); err != nil {
		t.Fatal("fixture bearer invalid", err)
	}
	reads.Store(0)
	sessions := session()
	ctx := context.WithValue(context.Background(), oauth2.HTTPClient, &http.Client{Transport: discoveryRoundTrip{}})
	login, err := weblogin.New(ctx, weblogin.Config{ClientID: "id", ClientSecret: "configured", PublicURL: "http://localhost", Issuer: "https://issuer.test", Store: &routeLoginStore{}, Sessions: sessions})
	if err != nil {
		t.Fatal(err)
	}
	manager := publicsummary.NewManager(func(context.Context) (publicsummary.Release, error) {
		return publicsummary.Release{Database: "corpus_vn_20990101", ID: "20990101", Projection: 1}, nil
	}, func(context.Context, string) (publicsummary.Pool, error) { return summaryRoutePool{}, nil }, clock)
	defer manager.Close()
	summaries := publicsummary.NewHandler(manager, true, clock)
	limiter := &routeLimiter{}
	mux := newWebMux(webDeps{summaries: summaries, store: routeAccountStore{}, sessions: sessions, login: login, verifier: verifier(), tools: mcpapi.Config{Limiter: limiter, Resolver: summaryReaderResolver{readerCalls}, Clock: clock}, loginLimit: newLoginLimiter(clock, []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")})})
	contract, err := contracttest.Load()
	if err != nil {
		t.Fatal(err)
	}
	coverage := contracttest.NewCoverage(contract, []string{"/api/v1/public/documents", "/api/v1/public/documents/{key}"})
	for _, path := range []string{"/api/v1/public/documents", "/api/v1/public/documents/vn:vbpl:1"} {
		var expected []byte
		for i := range 72 {
			request := httptest.NewRequest("GET", path, nil)
			if i%3 == 1 {
				request.AddCookie(&http.Cookie{Name: "__Host-session", Value: strings.Repeat("A", 43)})
			}
			if i%3 == 2 {
				request.Header.Set("Authorization", "Bearer trc_"+strings.Repeat("A", 43))
			}
			request.RemoteAddr = "10.0.0.1:1234"
			request.Header.Set("X-Forwarded-For", "invalid forwarded address")
			response := httptest.NewRecorder()
			mux.ServeHTTP(response, request)
			if response.Code != 200 {
				t.Fatalf("credential %d: %d %s", i%3, response.Code, response.Body.String())
			}
			schemaPath := path
			if path != "/api/v1/public/documents" {
				schemaPath = "/api/v1/public/documents/{key}"
			}
			coverage.Check(t, "GET", schemaPath, response)
			if i == 0 {
				expected = bytes.Clone(response.Body.Bytes())
			} else if !bytes.Equal(expected, response.Body.Bytes()) {
				t.Fatal("credentials changed summary")
			}
			if len(response.Result().Cookies()) != 0 {
				t.Fatal("summary set cookie")
			}
		}
	}
	if reads.Load() != 0 || readerCalls.Load() != 0 || len(limiter.calls) != 0 {
		t.Fatalf("summary crossed boundary: auth=%d reader=%d buckets=%v", reads.Load(), readerCalls.Load(), limiter.calls)
	}
}
