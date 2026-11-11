//go:build integration

package app

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/credential"
	"danny.vn/trachuan/apps/server/internal/limits"
	"danny.vn/trachuan/apps/server/internal/mcpapi"
	"danny.vn/trachuan/apps/server/internal/migrate"
	"danny.vn/trachuan/apps/server/internal/store"
	"danny.vn/trachuan/apps/server/internal/weblogin"
	"danny.vn/trachuan/apps/server/internal/weblogin/oidctest"
	"danny.vn/trachuan/apps/server/internal/websession"
	"danny.vn/trachuan/internal/retrieval"
	"danny.vn/trachuan/internal/testpg"
	"github.com/jackc/pgx/v5/pgxpool"
)

type e2eFixture struct {
	t        *testing.T
	server   *httptest.Server
	pool     *pgxpool.Pool
	sessions *websession.Manager
	verifier *credential.Verifier
	db       credentialStore
	offset   *atomic.Int64
}

func newE2E(t *testing.T) *e2eFixture {
	t.Helper()
	dsn := testpg.NewDatabase(t)
	if err := migrate.Up(t.Context(), dsn); err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.New(t.Context(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	provider := oidctest.New(t)
	provider.EnableLogin()
	server := httptest.NewUnstartedServer(nil)
	base := "https://" + server.Listener.Addr().String()
	q := store.New(pool)
	db := credentialStore{Queries: q, db: pool}
	offset := &atomic.Int64{}
	clock := func() time.Time { return time.Now().Add(time.Duration(offset.Load())) }
	sessions := &websession.Manager{Store: db, PublicURL: base, Clock: clock}
	verifier := &credential.Verifier{Store: q, Audience: base + "/mcp", Clock: clock}
	login, err := weblogin.New(t.Context(), weblogin.Config{ClientID: oidctest.ClientID, ClientSecret: "fixture", PublicURL: base, Issuer: provider.URL, Store: loginStore{q}, Sessions: sessions})
	if err != nil {
		t.Fatal(err)
	}
	mux := newWebMux(webDeps{store: db, sessions: sessions, login: login, verifier: verifier, tools: mcpapi.Config{Resolver: e2eResolver{}, Limiter: limits.New(time.Now)}, loginLimit: newLoginLimiter(time.Now, nil), oauthDB: pool, publicURL: base})
	mux.HandleFunc("GET /destination", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	server.Config.Handler = mux
	server.StartTLS()
	t.Cleanup(server.Close)
	return &e2eFixture{t, server, pool, sessions, verifier, db, offset}
}
func (f *e2eFixture) client() *http.Client {
	f.t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		f.t.Fatal(err)
	}
	client := *f.server.Client()
	c := &client
	c.Jar = jar
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	c.Timeout = 5 * time.Second
	return c
}
func (f *e2eFixture) request(c *http.Client, method, target, body, contentType, csrf, bearer string) *http.Response {
	f.t.Helper()
	if strings.HasPrefix(target, "/") {
		target = f.server.URL + target
	}
	req, err := http.NewRequestWithContext(f.t.Context(), method, target, strings.NewReader(body))
	if err != nil {
		f.t.Fatal("create request")
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if csrf != "" {
		req.Header.Set("Origin", f.server.URL)
		req.Header.Set("X-CSRF-Token", csrf)
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	if target == f.server.URL+"/mcp" {
		req.Header.Set("Accept", "application/json, text/event-stream")
	}
	resp, err := c.Do(req)
	if err != nil {
		f.t.Fatal("HTTP request failed")
	}
	return resp
}
func (f *e2eFixture) status(resp *http.Response, want int) {
	f.t.Helper()
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != want {
		f.t.Fatalf("status %d, want %d", resp.StatusCode, want)
	}
}
func (f *e2eFixture) json(resp *http.Response, want int, out any) {
	f.t.Helper()
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != want {
		f.t.Fatalf("status %d, want %d", resp.StatusCode, want)
	}
	if !strings.HasPrefix(resp.Header.Get("Content-Type"), "application/json") {
		f.t.Fatal("missing JSON content type")
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		f.t.Fatal("invalid JSON")
	}
}
func (f *e2eFixture) login(c *http.Client) {
	f.t.Helper()
	r := f.request(c, "GET", "/api/v1/auth/google/start?next=%2Fdestination", "", "", "", "")
	loc := r.Header.Get("Location")
	f.status(r, 302)
	r = f.request(c, "GET", loc, "", "", "", "")
	loc = r.Header.Get("Location")
	f.status(r, 302)
	r = f.request(c, "GET", loc, "", "", "", "")
	if r.Header.Get("Location") != "/destination" {
		f.t.Fatal("incorrect login destination")
	}
	found := false
	for _, ck := range r.Cookies() {
		if ck.Name == "__Host-session" {
			found = true
			if !ck.Secure || !ck.HttpOnly || ck.SameSite != http.SameSiteLaxMode || ck.Path != "/" || ck.MaxAge < 30*24*60*60-1 || ck.MaxAge > 30*24*60*60 {
				f.t.Fatal("session cookie attributes")
			}
		}
	}
	if !found {
		f.t.Fatal("session cookie missing")
	}
	f.status(r, 302)
	f.status(f.request(c, "GET", "/destination", "", "", "", ""), 204)
}

type e2eMe struct {
	ID    int64  `json:"id"`
	Email string `json:"email"`
	CSRF  string `json:"csrf_token"`
}

func (f *e2eFixture) me(c *http.Client) e2eMe {
	f.t.Helper()
	var me e2eMe
	f.json(f.request(c, "GET", "/api/v1/me", "", "", "", ""), 200, &me)
	if me.ID <= 0 || me.Email == "" || me.CSRF == "" {
		f.t.Fatal("invalid account shape")
	}
	return me
}
func (f *e2eFixture) mcp(c *http.Client, token string, want int) {
	f.t.Helper()
	r := f.request(c, "POST", "/mcp", `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`, "application/json", "", token)
	if want == 401 {
		if !strings.Contains(r.Header.Get("WWW-Authenticate"), `error="invalid_token"`) {
			f.t.Fatal("missing invalid token challenge")
		}
		f.status(r, want)
		return
	}
	var out struct {
		Result struct {
			Tools []json.RawMessage `json:"tools"`
		} `json:"result"`
	}
	f.json(r, want, &out)
	if len(out.Result.Tools) == 0 {
		f.t.Fatal("missing tools")
	}
}
func discard(r *http.Response) { _, _ = io.Copy(io.Discard, r.Body); _ = r.Body.Close() }

type e2eResolver struct{}

func (e2eResolver) Resolve(context.Context, string, string) (mcpapi.Lease, error) {
	return mcpapi.Lease{Engine: e2eEngine{}, Release: "20991111", Done: func() {}}, nil
}

type e2eEngine struct{}

func (e2eEngine) Search(context.Context, retrieval.Request) (retrieval.Response, error) {
	return retrieval.Response{}, nil
}
func (e2eEngine) Present(context.Context, retrieval.Response, retrieval.Detail) (retrieval.SearchResult, error) {
	return retrieval.SearchResult{}, nil
}
func (e2eEngine) Read(context.Context, retrieval.ReadRequest) (retrieval.ReadResult, error) {
	return retrieval.ReadResult{}, nil
}
func (e2eEngine) Status(context.Context) (retrieval.CorpusStatus, error) {
	return retrieval.CorpusStatus{}, nil
}
func (e2eEngine) Gaps(context.Context, string, int64, int) ([]retrieval.Gap, int64, error) {
	return nil, 0, nil
}
