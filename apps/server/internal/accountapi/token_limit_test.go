package accountapi

import (
	"context"
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/contracttest"
	"danny.vn/trachuan/apps/server/internal/credential"
	"danny.vn/trachuan/apps/server/internal/limits"
	"danny.vn/trachuan/apps/server/internal/store"
	"danny.vn/trachuan/apps/server/internal/websession"
	"github.com/jackc/pgx/v5"
)

type fullTokenStore struct{ *fakeStore }

func (*fullTokenStore) CreatePersonalToken(context.Context, store.CreatePersonalTokenParams) (store.PersonalToken, error) {
	return store.PersonalToken{}, credential.ErrTokenLimit
}
func TestTokens_LimitContract(t *testing.T) {
	c, err := contracttest.Load()
	if err != nil {
		t.Fatal(err)
	}
	testTokensLimitContract(t, contracttest.NewCoverage(c, []string{"/api/v1/me/tokens"}))
}
func testTokensLimitContract(t *testing.T, coverage *contracttest.Coverage) {
	_, s, v, raw, secret := setup(t)
	m := &websession.Manager{Store: s, Clock: func() time.Time { return s.now }, PublicURL: "https://example.invalid"}
	h := m.Middleware(m.CSRF(New(Config{Store: &fullTokenStore{s}, Sessions: m, Verifier: v, Limiter: limits.New(func() time.Time { return s.now })})))
	w := call(h, raw, secret, "POST", "/api/v1/me/tokens", `{"name":"test"}`)
	expect(t, w, 409)
	if !strings.Contains(w.Body.String(), `"code":"token_limit"`) || !strings.Contains(w.Body.String(), "Token limit reached.") {
		t.Fatal(w.Body.String())
	}
	coverage.Check(t, "POST", "/api/v1/me/tokens", w)
}

type noListStore struct{ *fakeStore }

func (*noListStore) ListTokens(context.Context, int64) ([]store.PersonalToken, error) {
	return nil, pgx.ErrNoRows
}
func TestTokens_RevokeWithoutListing(t *testing.T) {
	_, s, v, raw, secret := setup(t)
	token, digest, err := credential.Issue(strings.NewReader(strings.Repeat("x", 32)))
	if err != nil {
		t.Fatal(err)
	}
	s.tokens = append(s.tokens, store.PersonalToken{ID: 1, AccountID: 1, TokenHash: digest, ExpiresAt: s.now.Add(time.Hour)})
	m := &websession.Manager{Store: s, Clock: func() time.Time { return s.now }, PublicURL: "https://example.invalid"}
	h := m.Middleware(m.CSRF(New(Config{Store: &noListStore{s}, Sessions: m, Verifier: v, Limiter: limits.New(func() time.Time { return s.now })})))
	if _, err := v.Verify(context.Background(), token); err != nil {
		t.Fatal(err)
	}
	expect(t, call(h, raw, secret, "DELETE", "/api/v1/me/tokens/1", ""), 204)
	if _, err := v.Verify(context.Background(), token); err == nil {
		t.Fatal("revoked token cached")
	}
}
