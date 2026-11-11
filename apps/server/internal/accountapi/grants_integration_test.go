//go:build integration

package accountapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/credential"
	"danny.vn/trachuan/apps/server/internal/limits"
	"danny.vn/trachuan/apps/server/internal/migrate"
	"danny.vn/trachuan/apps/server/internal/store"
	"danny.vn/trachuan/apps/server/internal/websession"
	"danny.vn/trachuan/internal/testpg"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestGrants_PostgreSQLBoundsOwnershipAndEviction(t *testing.T) {
	dsn := testpg.NewDatabase(t)
	if err := migrate.Up(t.Context(), dsn); err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.New(t.Context(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	q := store.New(pool)
	a, err := q.CreateAccount(t.Context(), store.CreateAccountParams{GoogleSub: "grant-fixture", Email: "fixture@example.invalid"})
	if err != nil {
		t.Fatal(err)
	}
	other, err := q.CreateAccount(t.Context(), store.CreateAccountParams{GoogleSub: "other-grant-fixture", Email: "other@example.invalid"})
	if err != nil {
		t.Fatal(err)
	}
	var own, foreign store.OauthGrant
	for i := range 103 {
		client := fmt.Sprintf("https://client%d.invalid/meta", i)
		if _, err = q.CreateClient(t.Context(), store.CreateClientParams{ClientID: client, Kind: "cimd", Name: "Fixture", RedirectUris: []string{"https://callback.invalid/"}, GrantTypes: []string{"authorization_code", "refresh_token"}}); err != nil {
			t.Fatal(err)
		}
		accountID := a.ID
		if i == 102 {
			accountID = other.ID
		}
		grant, e := q.UpsertGrant(t.Context(), store.UpsertGrantParams{AccountID: accountID, ClientID: client, Resource: "https://example.invalid/mcp", Scope: "mcp:read"})
		if e != nil {
			t.Fatal(e)
		}
		// All timestamps tie, so the ID decides newest-first ordering.
		if _, err = pool.Exec(t.Context(), `UPDATE oauth_grant SET created_at='2026-01-01T00:00:00Z' WHERE id=$1`, grant.ID); err != nil {
			t.Fatal(err)
		}
		if i == 101 {
			own = grant
		}
		if i == 102 {
			foreign = grant
		}
	}
	session := &websession.Manager{Store: q, PublicURL: "https://example.invalid"}
	w := httptest.NewRecorder()
	if err = session.CreateSession(t.Context(), w, httptest.NewRequest("GET", "https://example.invalid/", nil), a.ID); err != nil {
		t.Fatal(err)
	}
	raw := w.Result().Cookies()[0].Value
	sessionRow, err := q.GetSession(t.Context(), hash(raw))
	if err != nil {
		t.Fatal(err)
	}
	access := "tra_" + strings.Repeat("A", 43)
	_, err = q.CreateTokenFamily(t.Context(), store.CreateTokenFamilyParams{TokenHash: hash(access), Kind: "access", GrantID: own.ID, FamilyID: strings.Repeat("a", 64), Resource: own.Resource, Scope: "mcp:read", AuthEpoch: a.AuthEpoch, ExpiresAt: time.Now().Add(time.Hour), FamilyExpiresAt: time.Now().Add(24 * time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	v := &credential.Verifier{Store: q, Audience: own.Resource}
	if _, err = v.Verify(t.Context(), access); err != nil {
		t.Fatal(err)
	}
	h := session.Middleware(session.CSRF(New(Config{Store: q, Sessions: session, Verifier: v, Limiter: limits.New(time.Now)})))
	w = call(h, raw, sessionRow.CsrfSecret, "GET", "/api/v1/me/grants", "")
	expect(t, w, 200)
	var rows []grantResponse
	if err = json.Unmarshal(w.Body.Bytes(), &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 100 || rows[0].ID != own.ID || rows[0].ClientIDHost != "client101.invalid" {
		t.Fatal("list bound, ownership or order mismatch")
	}
	for i := 1; i < len(rows); i++ {
		if rows[i-1].ID <= rows[i].ID {
			t.Fatal("unstable order")
		}
	}
	expect(t, call(h, raw, sessionRow.CsrfSecret, "DELETE", fmt.Sprintf("/api/v1/me/grants/%d", foreign.ID), ""), 404)
	if _, err = v.Verify(t.Context(), access); err != nil {
		t.Fatal("other account revoke evicted own token")
	}
	expect(t, call(h, raw, sessionRow.CsrfSecret, "DELETE", fmt.Sprintf("/api/v1/me/grants/%d", own.ID), ""), 204)
	if _, err = v.Verify(t.Context(), access); !errors.Is(err, credential.ErrUnauthorized) {
		t.Fatal("own grant cached after revoke")
	}
	rowsDB, err := q.ListGrants(t.Context(), other.ID)
	if err != nil || len(rowsDB) != 1 || rowsDB[0].ID != foreign.ID {
		t.Fatal("other account grant changed")
	}
}
