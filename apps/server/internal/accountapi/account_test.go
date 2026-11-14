package accountapi

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/credential"
	"danny.vn/trachuan/apps/server/internal/limits"
	"danny.vn/trachuan/apps/server/internal/store"
	"danny.vn/trachuan/apps/server/internal/websession"
	"github.com/jackc/pgx/v5"
)

type fakeStore struct {
	account  store.Account
	sessions []store.WebSession
	tokens   []store.PersonalToken
	now      time.Time
	deleted  bool
}

func (s *fakeStore) GetAccount(_ context.Context, id int64) (store.Account, error) {
	if id != s.account.ID {
		return store.Account{}, pgx.ErrNoRows
	}
	return s.account, nil
}
func (s *fakeStore) ListLiveSessions(_ context.Context, id int64) ([]store.WebSession, error) {
	var rows []store.WebSession
	for _, r := range s.sessions {
		if r.AccountID == id && r.RevokedAt == nil && r.ExpiresAt.After(s.now) && r.CreatedAt.Add(90*24*time.Hour).After(s.now) {
			rows = append(rows, r)
		}
	}
	return rows, nil
}
func (s *fakeStore) ListTokens(_ context.Context, id int64) ([]store.PersonalToken, error) {
	var rows []store.PersonalToken
	for _, r := range s.tokens {
		if r.AccountID == id {
			rows = append(rows, r)
		}
	}
	return rows, nil
}
func (s *fakeStore) CreateWebPersonalToken(_ context.Context, params store.WebPersonalTokenParams) (store.PersonalToken, error) {
	p := params.Token
	r := store.PersonalToken{ID: int64(len(s.tokens) + 1), AccountID: p.AccountID, TokenHash: p.TokenHash, Name: p.Name, ExpiresAt: p.ExpiresAt, CreatedAt: s.now, AuthEpoch: p.AuthEpoch}
	s.tokens = append(s.tokens, r)
	return r, nil
}
func (s *fakeStore) RevokeOwnToken(_ context.Context, p store.RevokeOwnTokenParams) (string, error) {
	for i := range s.tokens {
		r := &s.tokens[i]
		if r.ID == p.ID && r.AccountID == p.AccountID && r.RevokedAt == nil {
			r.RevokedAt = &s.now
			return r.TokenHash, nil
		}
	}
	return "", pgx.ErrNoRows
}
func (s *fakeStore) RequestDeletion(_ context.Context, id int64) (store.RequestDeletionRow, error) {
	if id != s.account.ID {
		return store.RequestDeletionRow{}, pgx.ErrNoRows
	}
	s.deleted = true
	for i := range s.sessions {
		if s.sessions[i].AccountID == id {
			s.sessions[i].RevokedAt = &s.now
		}
	}
	for i := range s.tokens {
		if s.tokens[i].AccountID == id {
			s.tokens[i].RevokedAt = &s.now
		}
	}
	return store.RequestDeletionRow{ID: id}, nil
}
func (s *fakeStore) PurgeDeletedAccounts(_ context.Context, now time.Time) (int64, error) {
	if s.deleted && now.After(s.now.Add(7*24*time.Hour)) {
		s.account.ID = 0
		return 1, nil
	}
	return 0, nil
}
func (s *fakeStore) CreateSession(context.Context, store.CreateSessionParams) (store.CreateSessionRow, error) {
	panic("unused")
}
func (s *fakeStore) GetSession(_ context.Context, hash string) (store.GetSessionRow, error) {
	for _, r := range s.sessions {
		if r.TokenHash == hash {
			return store.GetSessionRow{ID: r.ID, TokenHash: hash, AccountID: r.AccountID, CsrfSecret: r.CsrfSecret, CreatedAt: r.CreatedAt, LastSeenAt: r.LastSeenAt, ExpiresAt: r.ExpiresAt, RevokedAt: r.RevokedAt}, nil
		}
	}
	return store.GetSessionRow{}, pgx.ErrNoRows
}
func (s *fakeStore) TouchSession(context.Context, string) (int64, error)  { return 0, nil }
func (s *fakeStore) RevokeSession(context.Context, string) (int64, error) { panic("unused") }
func (s *fakeStore) RevokeOwnSession(_ context.Context, p store.RevokeOwnSessionParams) (int64, error) {
	for i := range s.sessions {
		r := &s.sessions[i]
		if r.ID == p.ID && r.AccountID == p.AccountID && r.RevokedAt == nil {
			r.RevokedAt = &s.now
			return 1, nil
		}
	}
	return 0, nil
}
func (s *fakeStore) GetCredential(_ context.Context, hash string) (store.GetCredentialRow, error) {
	for _, r := range s.tokens {
		if r.TokenHash == hash {
			return store.GetCredentialRow{AccountID: r.AccountID, ExpiresAt: r.ExpiresAt, RevokedAt: r.RevokedAt}, nil
		}
	}
	return store.GetCredentialRow{}, pgx.ErrNoRows
}
func (s *fakeStore) TouchPersonalToken(context.Context, string) error { return nil }
func hash(raw string) string                                          { h := sha256.Sum256([]byte(raw)); return hex.EncodeToString(h[:]) }
func setup(t *testing.T) (http.Handler, *fakeStore, *credential.Verifier, string, string) {
	t.Helper()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	raw := base64.RawURLEncoding.EncodeToString(make([]byte, 32))
	secret := base64.RawURLEncoding.EncodeToString([]byte(strings.Repeat("s", 32)))
	s := &fakeStore{now: now, account: store.Account{ID: 1, Email: "test@example.invalid", CreatedAt: now}, sessions: []store.WebSession{{ID: 1, AccountID: 1, TokenHash: hash(raw), CsrfSecret: secret, CreatedAt: now, LastSeenAt: now, ExpiresAt: now.Add(time.Hour)}, {ID: 2, AccountID: 2}}}
	m := &websession.Manager{Store: s, Clock: func() time.Time { return now }, PublicURL: "https://example.invalid"}
	v := &credential.Verifier{Store: s, Clock: func() time.Time { return now }, Audience: "mcp"}
	h := New(Config{Store: s, Sessions: m, Verifier: v, Limiter: limits.New(func() time.Time { return now }), Clock: func() time.Time { return now }, Rand: strings.NewReader(strings.Repeat("a", 1024))})
	return m.Middleware(m.CSRF(h)), s, v, raw, secret
}
func call(h http.Handler, raw, secret, method, path, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "https://example.invalid"+path, strings.NewReader(body))
	r.AddCookie(&http.Cookie{Name: "__Host-session", Value: raw})
	r.Header.Set("Origin", "https://example.invalid")
	r.Header.Set("X-CSRF-Token", secret)
	if body != "" {
		r.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}
func expect(t *testing.T, w *httptest.ResponseRecorder, status int) {
	t.Helper()
	if w.Code != status {
		t.Fatalf("status %d, want %d: %s", w.Code, status, w.Body.String())
	}
}
func TestMe_ReturnsAccountAndCSRF(t *testing.T) {
	h, _, _, r, c := setup(t)
	w := call(h, r, c, "GET", "/api/v1/me", "")
	expect(t, w, 200)
	if !strings.Contains(w.Body.String(), c) || !strings.Contains(w.Body.String(), "test@example.invalid") {
		t.Fatal("account or csrf missing")
	}
}
func TestMe_NoCookie(t *testing.T) {
	for _, cookie := range []string{"", "other=value; language=en"} {
		h, _, _, _, _ := setup(t)
		r := httptest.NewRequest(http.MethodGet, "https://example.invalid/api/v1/me", nil)
		if cookie != "" {
			r.Header.Set("Cookie", cookie)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		expect(t, w, 204)
		if w.Body.Len() != 0 || w.Header().Get("Cache-Control") != "no-store" || len(w.Result().Cookies()) != 0 {
			t.Fatalf("unexpected response: headers=%v body=%q", w.Header(), w.Body.String())
		}
	}
}
func TestMe_InvalidCookies(t *testing.T) {
	for _, tc := range []struct{ name, cookie string }{
		{"empty", "__Host-session="},
		{"space before empty", "__Host-session ="},
		{"space before invalid", "__Host-session =bad"},
		{"malformed", "__Host-session=\"bad value\""},
		{"unparseable", "other=value; __Host-session =\"unterminated"},
		{"duplicate", "__Host-session=bad; __Host-session=bad"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, _, _, _, _ := setup(t)
			r := httptest.NewRequest(http.MethodGet, "https://example.invalid/api/v1/me", nil)
			r.Header.Set("Cookie", tc.cookie)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			expect(t, w, 401)
		})
	}
	for _, state := range []string{"expired", "revoked"} {
		t.Run(state, func(t *testing.T) {
			h, s, _, raw, secret := setup(t)
			if state == "expired" {
				s.sessions[0].ExpiresAt = s.now
			} else {
				s.sessions[0].RevokedAt = &s.now
			}
			expect(t, call(h, raw, secret, "GET", "/api/v1/me", ""), 401)
		})
	}
}
func TestSessions_ListAndRevokeOwn(t *testing.T) {
	h, _, _, r, c := setup(t)
	expect(t, call(h, r, c, "GET", "/api/v1/me/sessions", ""), 200)
	expect(t, call(h, r, c, "DELETE", "/api/v1/me/sessions/1", ""), 204)
	expect(t, call(h, r, c, "GET", "/api/v1/me", ""), 401)
}
func TestTokens_CreateShowsOnceListRevoke(t *testing.T) {
	h, s, _, r, c := setup(t)
	w := call(h, r, c, "POST", "/api/v1/me/tokens", `{"name":" test "}`)
	expect(t, w, 201)
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	token, ok := body["token"].(string)
	if !ok || !strings.HasPrefix(token, "trc_") {
		t.Fatal("token missing")
	}
	if s.tokens[0].Name != "test" || !s.tokens[0].ExpiresAt.Equal(s.now.Add(90*24*time.Hour)) {
		t.Fatal("default lifetime or name")
	}
	w = call(h, r, c, "GET", "/api/v1/me/tokens", "")
	expect(t, w, 200)
	if strings.Contains(w.Body.String(), token) || strings.Contains(w.Body.String(), s.tokens[0].TokenHash) {
		t.Fatal("token disclosed")
	}
	expect(t, call(h, r, c, "DELETE", "/api/v1/me/tokens/1", ""), 204)
	for _, b := range []string{`{"name":""}`, `{"name":"x","days":366}`, `{"name":"x","days":0}`, `{"name":"x\u0000"}`, `{"name":"` + strings.Repeat("x", 65) + `"}`} {
		expect(t, call(h, r, c, "POST", "/api/v1/me/tokens", b), 400)
	}
}
func TestTokens_RevokeEvictsCredentialCache(t *testing.T) {
	h, _, v, r, c := setup(t)
	w := call(h, r, c, "POST", "/api/v1/me/tokens", `{"name":"test"}`)
	var b map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &b)
	token := b["token"].(string)
	if _, err := v.Verify(context.Background(), token); err != nil {
		t.Fatal(err)
	}
	expect(t, call(h, r, c, "DELETE", "/api/v1/me/tokens/1", ""), 204)
	if _, err := v.Verify(context.Background(), token); err == nil {
		t.Fatal("revoked token accepted")
	}
}
func TestAccount_CrossAccountIDsAre404(t *testing.T) {
	h, s, _, r, c := setup(t)
	s.tokens = append(s.tokens, store.PersonalToken{ID: 5, AccountID: 2})
	for _, p := range []string{"/api/v1/me/sessions/2", "/api/v1/me/sessions/99", "/api/v1/me/tokens/5", "/api/v1/me/tokens/99"} {
		expect(t, call(h, r, c, "DELETE", p, ""), 404)
	}
}
func TestDelete_BlocksRevokesAndPurgesAfterSevenDays(t *testing.T) {
	h, s, _, r, c := setup(t)
	expect(t, call(h, r, c, "DELETE", "/api/v1/me", ""), 204)
	if !s.deleted {
		t.Fatal("deletion not requested")
	}
	expect(t, call(h, r, c, "GET", "/api/v1/me", ""), 401)
	n, err := Purge(context.Background(), s, s.now.Add(6*24*time.Hour))
	if err != nil || n != 0 {
		t.Fatal("purged early")
	}
	n, err = Purge(context.Background(), s, s.now.Add(8*24*time.Hour))
	if err != nil || n != 1 {
		t.Fatal("purge failed")
	}
}
func TestAccount_LimitsAndCSRF(t *testing.T) {
	h, _, _, r, c := setup(t)
	expect(t, call(h, r, "wrong", "DELETE", "/api/v1/me", ""), 403)
	for range 120 {
		expect(t, call(h, r, c, "GET", "/api/v1/me", ""), 200)
	}
	w := call(h, r, c, "GET", "/api/v1/me", "")
	expect(t, w, 429)
	if w.Header().Get("Retry-After") == "" {
		t.Fatal("missing retry-after")
	}
}

func TestAccount_ResponseFieldsAndTokenBounds(t *testing.T) {
	h, s, _, r, c := setup(t)
	w := call(h, r, c, "GET", "/api/v1/me/sessions", "")
	expect(t, w, 200)
	var sessions []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &sessions); err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 || sessions[0]["id"] != float64(1) || sessions[0]["current"] != true || sessions[0]["created_at"] == nil || sessions[0]["last_seen_at"] == nil || strings.Contains(w.Body.String(), hash(r)) || strings.Contains(w.Body.String(), c) {
		t.Fatal("incorrect session response")
	}
	for _, tc := range []struct {
		name string
		days int
	}{{"a", 1}, {strings.Repeat("é", 64), 365}} {
		body, _ := json.Marshal(map[string]any{"name": tc.name, "days": tc.days})
		expect(t, call(h, r, c, "POST", "/api/v1/me/tokens", string(body)), 201)
		if !s.tokens[len(s.tokens)-1].ExpiresAt.Equal(s.now.Add(time.Duration(tc.days) * 24 * time.Hour)) {
			t.Fatal("wrong expiry")
		}
	}
}
