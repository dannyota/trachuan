package oauth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/credential"
	"danny.vn/trachuan/apps/server/internal/store"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type revokeDB struct{ *tokenMemory }

func (m revokeDB) Begin(ctx context.Context) (pgx.Tx, error) {
	tx, err := m.tokenMemory.Begin(ctx)
	if err != nil {
		return nil, err
	}
	return &revokeTx{tokenMemoryTx: tx.(*tokenMemoryTx)}, nil
}

type revokeTx struct{ *tokenMemoryTx }

func (tx *revokeTx) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	if strings.Contains(sql, "name: RevokeToken") {
		if err := tx.check(ctx, sql); err != nil {
			return pgconn.CommandTag{}, err
		}
		row, ok := tx.tokens[args[0].(string)]
		if !ok {
			return pgconn.NewCommandTag("UPDATE 0"), nil
		}
		now := time.Now()
		row.RevokedAt = &now
		tx.tokens[args[0].(string)] = row
		return pgconn.NewCommandTag("UPDATE 1"), nil
	}
	return tx.tokenMemoryTx.Exec(ctx, sql, args...)
}
func revokeFixture() (*tokenMemory, string, string) {
	m := newTokenMemory()
	access := "tra_" + strings.Repeat("A", 43)
	refresh := "trr_" + strings.Repeat("B", 43)
	for _, v := range []struct{ raw, kind string }{{access, "access"}, {refresh, "refresh"}} {
		hash := tokenDigest(v.raw)
		m.tokens[hash] = store.GetTokenRow{TokenHash: hash, Kind: v.kind, AccountID: 1, GrantID: 2, ClientID: "client", Scope: "mcp:read", Resource: tokenTestBase + "/mcp", AuthEpoch: 7, AccountAuthEpoch: 7, ExpiresAt: time.Now().Add(time.Hour), FamilyExpiresAt: time.Now().Add(24 * time.Hour)}
	}
	return m, access, refresh
}
func revokeCall(h http.Handler, raw, client string) *httptest.ResponseRecorder {
	r := httptest.NewRequest("POST", "/oauth/revoke", strings.NewReader(url.Values{"token": {raw}, "client_id": {client}, "token_type_hint": {"ignored"}}.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}
func TestRevoke_ClientMismatchRefused(t *testing.T) {
	m, access, refresh := revokeFixture()
	h := NewRevokeHandler(revokeDB{m}, func(string) { t.Error("evicted") }, func(int64) { t.Error("evicted grant") })
	for _, raw := range []string{access, refresh} {
		tokenJSON(t, revokeCall(h, raw, "other"), 400, "invalid_request")
	}
	if m.revoked || m.commits != 0 {
		t.Fatal("mismatch changed state")
	}
}
func TestRevoke_UnknownIs200(t *testing.T) {
	m, _, _ := revokeFixture()
	w := revokeCall(NewRevokeHandler(revokeDB{m}, nil, nil), "unknown", "client")
	if w.Code != 200 || w.Body.Len() != 0 || m.revoked {
		t.Fatalf("unknown status %d", w.Code)
	}
}

type revokeCredentialStore struct{ *tokenMemory }

func (s revokeCredentialStore) GetCredential(context.Context, string) (store.GetCredentialRow, error) {
	return store.GetCredentialRow{}, pgx.ErrNoRows
}
func (s revokeCredentialStore) TouchPersonalToken(context.Context, string) error { return nil }
func (s revokeCredentialStore) TouchOAuthGrant(context.Context, int64) error     { return nil }
func (s revokeCredentialStore) GetToken(_ context.Context, hash string) (store.GetTokenRow, error) {
	row, ok := s.tokens[hash]
	if !ok {
		return row, pgx.ErrNoRows
	}
	return row, nil
}
func TestRevoke_RefreshRevokesGrantAndEvictsCache(t *testing.T) {
	m, access, refresh := revokeFixture()
	v := &credential.Verifier{Store: revokeCredentialStore{m}, Audience: tokenTestBase + "/mcp"}
	if _, err := v.Verify(t.Context(), access); err != nil {
		t.Fatal(err)
	}
	h := NewRevokeHandler(revokeDB{m}, v.Evict, v.EvictGrant)
	if w := revokeCall(h, refresh, "client"); w.Code != 200 {
		t.Fatal(w.Code)
	}
	if !m.revoked || m.code.RevokedAt == nil {
		t.Fatal("grant/code live")
	}
	if _, err := v.Verify(t.Context(), access); !errors.Is(err, credential.ErrUnauthorized) {
		t.Fatal("cached access accepted")
	}
}
func TestRevoke_AccessOnlyAndFailures(t *testing.T) {
	for _, phase := range []string{"success", "begin", "lookup", "account", "update", "commit"} {
		t.Run(phase, func(t *testing.T) {
			m, access, refresh := revokeFixture()
			switch phase {
			case "begin":
				m.beginErr = errors.New("db")
			case "lookup":
				m.fail = "name: GetToken"
			case "account":
				m.fail = "name: LockCredentialAccount"
			case "update":
				m.fail = "name: RevokeToken"
			case "commit":
				m.commitErr = errors.New("db")
			}
			evicted := false
			h := NewRevokeHandler(revokeDB{m}, func(hash string) {
				if hash != tokenDigest(access) {
					t.Error("wrong eviction")
				}
				evicted = true
			}, func(int64) { t.Error("grant eviction for access") })
			w := revokeCall(h, access, "client")
			if phase == "success" {
				if w.Code != 200 {
					t.Fatal(w.Code)
				}
			} else {
				tokenJSON(t, w, 503, "temporarily_unavailable")
			}
			if m.revoked || m.tokens[tokenDigest(refresh)].RevokedAt != nil {
				t.Fatal("access revoked grant/refresh")
			}
			if evicted != (phase == "success" || phase == "commit") {
				t.Fatal("eviction on wrong outcome")
			}
			if phase == "success" && m.tokens[tokenDigest(access)].RevokedAt == nil {
				t.Fatal("access still live")
			}
		})
	}
}
func TestRevoke_PublicClientAndFormBounds(t *testing.T) {
	for _, tc := range []struct {
		body, auth, media string
		status            int
		code              string
	}{
		{"token=x", "", "application/x-www-form-urlencoded", 400, "invalid_request"},
		{"client_id=client", "", "application/x-www-form-urlencoded", 400, "invalid_request"},
		{"token=x&client_id=client&token=x", "", "application/x-www-form-urlencoded", 400, "invalid_request"},
		{"token=x&client_id=client&client_secret=", "", "application/x-www-form-urlencoded", 401, "invalid_client"},
		{"token=x&client_id=client", "Basic fixture", "application/x-www-form-urlencoded", 401, "invalid_client"},
		{"token=x&client_id=client", "", "application/json", 400, "invalid_request"},
		{strings.Repeat("x", 4097), "", "application/x-www-form-urlencoded", 400, "invalid_request"},
	} {
		m, _, _ := revokeFixture()
		r := httptest.NewRequest("POST", "/oauth/revoke", strings.NewReader(tc.body))
		r.Header.Set("Content-Type", tc.media)
		if tc.auth != "" {
			r.Header.Set("Authorization", tc.auth)
		}
		w := httptest.NewRecorder()
		NewRevokeHandler(revokeDB{m}, nil, nil).ServeHTTP(w, r)
		tokenJSON(t, w, tc.status, tc.code)
		if m.commits != 0 {
			t.Fatal("invalid request committed")
		}
	}
}
