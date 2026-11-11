package weblogin

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/weblogin/oidctest"
)

const callbackPath = "/api/v1/auth/google/callback"
const startPath = "/api/v1/auth/google/start"
const publicURL = "https://law.example"

type fakeStore struct {
	mu                             sync.Mutex
	transactions                   map[string]LoginTransaction
	claimed                        map[string]bool
	accounts                       map[string]GoogleAccount
	emails                         map[string]string
	createErr, claimErr, upsertErr error
	upserts, claims                int
}

func newFakeStore() *fakeStore {
	return &fakeStore{transactions: make(map[string]LoginTransaction), claimed: make(map[string]bool), accounts: make(map[string]GoogleAccount), emails: make(map[string]string)}
}
func (s *fakeStore) CreateLoginTransaction(_ context.Context, tx LoginTransaction) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.createErr != nil {
		return s.createErr
	}
	s.transactions[tx.HandleHash] = tx
	return nil
}
func (s *fakeStore) ClaimLoginTransaction(_ context.Context, p ClaimLoginTransactionParams) (LoginTransaction, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.claims++
	if s.claimErr != nil {
		return LoginTransaction{}, s.claimErr
	}
	tx, ok := s.transactions[p.HandleHash]
	if !ok || s.claimed[p.HandleHash] || !tx.ExpiresAt.After(p.Now) {
		return LoginTransaction{}, sql.ErrNoRows
	}
	s.claimed[p.HandleHash] = true
	return tx, nil
}
func (s *fakeStore) UpsertGoogleAccount(_ context.Context, p UpsertGoogleAccountParams) (GoogleAccount, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.upserts++
	if s.upsertErr != nil {
		return GoogleAccount{}, s.upsertErr
	}
	a, ok := s.accounts[p.Sub]
	if !ok {
		a = GoogleAccount{ID: int64(len(s.accounts) + 1)}
	}
	s.accounts[p.Sub] = a
	s.emails[p.Sub] = p.Email
	return a, nil
}

type fakeSessions struct {
	mu                       sync.Mutex
	accounts                 []int64
	presented                []string
	revokeErr, createErr     error
	revokeCalls, createCalls int
}

func (s *fakeSessions) CreateSession(_ context.Context, w http.ResponseWriter, r *http.Request, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if c, err := r.Cookie("__Host-session"); err == nil {
		s.revokeCalls++
		if s.revokeErr != nil {
			return s.revokeErr
		}
		s.presented = append(s.presented, c.Value)
	}
	s.createCalls++
	if s.createErr != nil {
		http.SetCookie(w, &http.Cookie{Name: "__Host-session", Value: "must-not-escape", Path: "/"})
		return s.createErr
	}
	s.accounts = append(s.accounts, id)
	http.SetCookie(w, &http.Cookie{Name: "__Host-session", Value: "new-test-session", Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: 2592000})
	return nil
}
func (s *fakeSessions) count() int { s.mu.Lock(); defer s.mu.Unlock(); return len(s.accounts) }

type fixture struct {
	h        *Handler
	p        *oidctest.Provider
	store    *fakeStore
	sessions *fakeSessions
	now      time.Time
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	f := &fixture{p: oidctest.New(t), store: newFakeStore(), sessions: &fakeSessions{}, now: time.Now().Truncate(time.Second)}
	var err error
	f.h, err = New(t.Context(), Config{ClientID: oidctest.ClientID, ClientSecret: "test-client-secret", PublicURL: publicURL, Issuer: f.p.URL, Store: f.store, Sessions: f.sessions, Clock: func() time.Time { return f.now }})
	if err != nil {
		t.Fatal(err)
	}
	return f
}

type attempt struct {
	cookie                  *http.Cookie
	state, nonce, challenge string
	tx                      LoginTransaction
}

func (f *fixture) begin(t *testing.T, next string) attempt {
	t.Helper()
	r := httptest.NewRequest(http.MethodGet, startPath+"?next="+url.QueryEscape(next), nil)
	r.Host = "attacker.example"
	r.Header.Set("X-Forwarded-Host", "attacker.example")
	r.Header.Set("X-Forwarded-Proto", "http")
	w := httptest.NewRecorder()
	f.h.Start.ServeHTTP(w, r)
	if w.Code != 302 {
		t.Fatalf("start status=%d body=%s", w.Code, w.Body.String())
	}
	u, err := url.Parse(w.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	c := cookieNamed(t, w, "__Host-login")
	sum := sha256.Sum256([]byte(c.Value))
	tx := f.store.transactions[hex.EncodeToString(sum[:])]
	return attempt{cookie: c, state: u.Query().Get("state"), nonce: u.Query().Get("nonce"), challenge: u.Query().Get("code_challenge"), tx: tx}
}
func (f *fixture) register(a attempt, code string, mutate func(*oidctest.Claims)) {
	c := oidctest.Claims{Subject: "test-sub", Email: "first@example.test", EmailVerified: true, Nonce: a.nonce, Challenge: a.challenge, RedirectURI: publicURL + callbackPath}
	if mutate != nil {
		mutate(&c)
	}
	f.p.RegisterCode(code, c)
}
func (f *fixture) callback(a attempt, code string) *httptest.ResponseRecorder {
	return f.callbackQuery(a, url.Values{"code": {code}, "state": {a.state}})
}
func (f *fixture) callbackQuery(a attempt, q url.Values) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodGet, callbackPath+"?"+q.Encode(), nil)
	if a.cookie != nil {
		r.AddCookie(a.cookie)
	}
	r.AddCookie(&http.Cookie{Name: "__Host-session", Value: "old-test-session"})
	w := httptest.NewRecorder()
	f.h.Callback.ServeHTTP(w, r)
	return w
}
func cookieNamed(t *testing.T, w *httptest.ResponseRecorder, name string) *http.Cookie {
	t.Helper()
	for _, c := range w.Result().Cookies() {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("missing cookie %s", name)
	return nil
}
func assertNoSession(t *testing.T, f *fixture, w *httptest.ResponseRecorder) {
	t.Helper()
	if f.sessions.count() != 0 {
		t.Error("session created on failure")
	}
	for _, c := range w.Result().Cookies() {
		if c.Name == "__Host-session" && c.MaxAge >= 0 {
			t.Error("session cookie issued on failure")
		}
	}
}
func assertRejected(t *testing.T, f *fixture, w *httptest.ResponseRecorder, code string) {
	t.Helper()
	if w.Code != 302 || w.Header().Get("Location") != "/login?error="+code {
		t.Errorf("response=%d %q body=%s", w.Code, w.Header().Get("Location"), w.Body.String())
	}
	if c := cookieNamed(t, w, "__Host-login"); c.MaxAge >= 0 || c.Value != "" || !c.Secure || !c.HttpOnly || c.Path != "/" || c.Domain != "" || c.SameSite != http.SameSiteLaxMode {
		t.Errorf("login cookie not cleared correctly: %+v", c)
	}
	assertNoSession(t, f, w)
}
func assertUnavailable(t *testing.T, f *fixture, w *httptest.ResponseRecorder) {
	t.Helper()
	if w.Code != 503 || w.Header().Get("Content-Type") != "application/json" || w.Body.String() != "{\"error\":{\"code\":\"unavailable\",\"message\":\"Service unavailable.\"}}\n" || w.Header().Get("Location") != "" {
		t.Errorf("response=%d %v %s", w.Code, w.Header(), w.Body.String())
	}
	assertNoSession(t, f, w)
}

func privateFailure() error { return errors.New("private-database-detail") }
