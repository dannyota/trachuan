package accountapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
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

type grantsStore struct {
	*fakeStore
	rows []store.ListGrantsRow
	err  error
}

func (s *grantsStore) ListGrants(ctx context.Context, id int64) ([]store.ListGrantsRow, error) {
	if _, ok := ctx.Deadline(); !ok {
		return nil, errors.New("missing deadline")
	}
	var out []store.ListGrantsRow
	for _, r := range s.rows {
		if r.AccountID == id && r.RevokedAt == nil {
			out = append(out, r)
		}
	}
	return out, s.err
}
func (s *grantsStore) RevokeOwnGrant(ctx context.Context, p store.RevokeOwnGrantParams) (store.RevokeOwnGrantRow, error) {
	if _, ok := ctx.Deadline(); !ok {
		return store.RevokeOwnGrantRow{}, errors.New("missing deadline")
	}
	for i := range s.rows {
		r := &s.rows[i]
		if r.AccountID == p.AccountID && r.ID == p.ID && r.RevokedAt == nil {
			r.RevokedAt = &s.now
			return store.RevokeOwnGrantRow{ID: r.ID, AccountID: r.AccountID}, s.err
		}
	}
	return store.RevokeOwnGrantRow{}, pgx.ErrNoRows
}

type grantEviction struct{ ids []int64 }

func (e *grantEviction) Evict(string)        {}
func (e *grantEviction) EvictAccount(int64)  {}
func (e *grantEviction) EvictGrant(id int64) { e.ids = append(e.ids, id) }
func grantsSetup(t *testing.T) (http.Handler, *grantsStore, *grantEviction, string, string) {
	t.Helper()
	_, s, _, raw, secret := setup(t)
	db := &grantsStore{fakeStore: s, rows: []store.ListGrantsRow{{ID: 1, AccountID: 1, ClientID: "https://client.invalid/meta", Name: "Fixture", CreatedAt: s.now, LastUsedAt: s.now}, {ID: 2, AccountID: 2}}}
	e := &grantEviction{}
	m := &websession.Manager{Store: s, Clock: func() time.Time { return s.now }, PublicURL: "https://example.invalid"}
	h := m.Middleware(m.CSRF(New(Config{Store: db, Sessions: m, Verifier: e, Limiter: limits.New(func() time.Time { return s.now })})))
	return h, db, e, raw, secret
}
func TestGrants_ListAndRevokeOwnOnly(t *testing.T) {
	h, s, e, raw, secret := grantsSetup(t)
	w := call(h, raw, secret, "GET", "/api/v1/me/grants", "")
	expect(t, w, 200)
	var rows []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["client_name"] != "Fixture" || rows[0]["client_id_host"] != "client.invalid" || rows[0]["created_at"] == nil || rows[0]["last_used_at"] == nil {
		t.Fatalf("grant view %v", rows)
	}
	for _, id := range []string{"2", "99", "bad"} {
		expect(t, call(h, raw, secret, "DELETE", "/api/v1/me/grants/"+id, ""), 404)
	}
	if len(e.ids) != 0 || s.rows[1].RevokedAt != nil {
		t.Fatal("cross account changed")
	}
	expect(t, call(h, raw, secret, "DELETE", "/api/v1/me/grants/1", ""), 204)
	if len(e.ids) != 1 || e.ids[0] != 1 || s.rows[0].RevokedAt == nil {
		t.Fatal("grant not revoked and evicted")
	}
	expect(t, call(h, raw, secret, "GET", "/api/v1/me/grants", ""), 200)
}
func TestGrants_FailureEvictsConservatively(t *testing.T) {
	h, s, e, raw, secret := grantsSetup(t)
	s.err = errors.New("unknown database outcome")
	expect(t, call(h, raw, secret, "GET", "/api/v1/me/grants", ""), 503)
	expect(t, call(h, raw, secret, "DELETE", "/api/v1/me/grants/1", ""), 503)
	if len(e.ids) != 1 || e.ids[0] != 1 {
		t.Fatal("unknown outcome left cached grant")
	}
}
func TestGrants_Contract(t *testing.T) {
	c, err := contracttest.Load()
	if err != nil {
		t.Fatal(err)
	}
	coverage := contracttest.NewCoverage(c, []string{"/api/v1/me/grants", "/api/v1/me/grants/{id}"})
	testGrantsContract(t, coverage)
	if missing := coverage.Missing(); len(missing) > 0 {
		t.Fatal(missing)
	}
}
func testGrantsContract(t *testing.T, coverage *contracttest.Coverage) {
	for _, method := range []string{"GET", "DELETE"} {
		path, actual := "/api/v1/me/grants", "/api/v1/me/grants"
		if method == "DELETE" {
			path += "/{id}"
			actual += "/1"
		}
		for _, scenario := range []string{"success", "unauthenticated", "csrf", "limit", "unavailable", "missing"} {
			if method == "GET" && (scenario == "csrf" || scenario == "missing") {
				continue
			}
			t.Run(method+scenario, func(t *testing.T) {
				h, s, _, raw, secret := grantsSetup(t)
				want := 200
				if method == "DELETE" {
					want = 204
				}
				switch scenario {
				case "unauthenticated":
					raw = "invalid"
					want = 401
				case "csrf":
					secret = "invalid"
					want = 403
				case "limit":
					for range 120 {
						expect(t, call(h, raw, secret, "GET", "/api/v1/me", ""), 200)
					}
					want = 429
				case "unavailable":
					s.err = errors.New("db")
					want = 503
				case "missing":
					s.rows = nil
					want = 404
				}
				w := call(h, raw, secret, method, actual, "")
				expect(t, w, want)
				coverage.Check(t, method, path, w)
			})
		}
	}
}

func TestGrants_BearerCannotAuthorize(t *testing.T) {
	h, _, _, _, _ := grantsSetup(t)
	r := httptest.NewRequest("GET", "/api/v1/me/grants", nil)
	r.Header.Set("Authorization", "Bearer tra_fixture")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	expect(t, w, 401)
}

type grantCredentialStore struct {
	*fakeStore
	token store.GetTokenRow
}

func (s *grantCredentialStore) GetToken(context.Context, string) (store.GetTokenRow, error) {
	row := s.token
	if s.deleted {
		row.GrantRevokedAt = &s.now
		row.BlockedAt = &s.now
		row.AccountAuthEpoch++
	}
	return row, nil
}
func (s *grantCredentialStore) TouchOAuthGrant(context.Context, int64) error { return nil }
func TestDelete_EvictsOAuthAccessTokens(t *testing.T) {
	_, s, _, raw, secret := setup(t)
	db := &grantCredentialStore{fakeStore: s, token: store.GetTokenRow{Kind: "access", AccountID: 1, GrantID: 3, Resource: "https://example.invalid/mcp", Scope: "mcp:read", ExpiresAt: s.now.Add(time.Hour), FamilyExpiresAt: s.now.Add(24 * time.Hour)}}
	v := &credential.Verifier{Store: db, Audience: db.token.Resource, Clock: func() time.Time { return s.now }}
	access := "tra_" + strings.Repeat("A", 43)
	if _, err := v.Verify(t.Context(), access); err != nil {
		t.Fatal(err)
	}
	m := &websession.Manager{Store: s, PublicURL: "https://example.invalid", Clock: func() time.Time { return s.now }}
	h := m.Middleware(m.CSRF(New(Config{Store: db, Sessions: m, Verifier: v, Limiter: limits.New(func() time.Time { return s.now })})))
	expect(t, call(h, raw, secret, "DELETE", "/api/v1/me", ""), 204)
	if _, err := v.Verify(t.Context(), access); !errors.Is(err, credential.ErrUnauthorized) {
		t.Fatal("deleted account's access token cached")
	}
}
