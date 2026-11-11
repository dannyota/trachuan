//go:build integration

package oauth

import (
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/migrate"
	"danny.vn/trachuan/apps/server/internal/store"
	"danny.vn/trachuan/internal/testpg"
	"github.com/jackc/pgx/v5/pgxpool"
)

type tokenPG struct {
	pool    *pgxpool.Pool
	account store.Account
	grant   store.OauthGrant
	handler http.Handler
}

func newTokenPG(t *testing.T) tokenPG {
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
	q := store.New(pool)
	account, err := q.CreateAccount(t.Context(), store.CreateAccountParams{GoogleSub: "token-fixture", Email: "fixture@example.invalid"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = q.CreateClient(t.Context(), store.CreateClientParams{ClientID: "client", Kind: "dcr", Name: "Fixture", RedirectUris: []string{"http://localhost/cb"}, GrantTypes: []string{"authorization_code", "refresh_token"}})
	if err != nil {
		t.Fatal(err)
	}
	grant, err := q.UpsertGrant(t.Context(), store.UpsertGrantParams{AccountID: account.ID, ClientID: "client", Resource: tokenTestBase + "/mcp", Scope: "mcp:read"})
	if err != nil {
		t.Fatal(err)
	}
	return tokenPG{pool: pool, account: account, grant: grant, handler: NewTokenHandler(pool, tokenTestBase, nil)}
}
func (p tokenPG) code(t *testing.T, raw string) {
	t.Helper()
	tx, err := p.pool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(t.Context()) }()
	if _, err = InUse(t.Context(), tx, "client"); err != nil {
		t.Fatal(err)
	}
	_, err = store.New(tx).CreateCode(t.Context(), store.CreateCodeParams{GrantID: p.grant.ID, CodeHash: tokenDigest(raw), RedirectUri: "http://localhost/cb", Resource: tokenTestBase + "/mcp", CodeChallenge: tokenChallenge(), Scope: "mcp:read", AuthEpoch: p.account.AuthEpoch, ExpiresAt: time.Now().Add(time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
}
func (p tokenPG) exchange(t *testing.T, raw string) map[string]any {
	t.Helper()
	p.code(t, raw)
	form := codeForm()
	form.Set("code", raw)
	return tokenJSON(t, requestToken(p.handler, form), 200, "")
}
func (p tokenPG) assertRevoked(t *testing.T) {
	t.Helper()
	var revoked, live bool
	err := p.pool.QueryRow(t.Context(), `SELECT revoked_at IS NOT NULL,
 EXISTS(SELECT FROM oauth_token WHERE grant_id=$1 AND revoked_at IS NULL)
 OR EXISTS(SELECT FROM oauth_code WHERE grant_id=$1 AND consumed_at IS NULL AND revoked_at IS NULL)
 FROM oauth_grant WHERE id=$1`, p.grant.ID).Scan(&revoked, &live)
	if err != nil || !revoked || live {
		t.Fatalf("grant revoked %v, live credentials %v, error %v", revoked, live, err)
	}
}

func TestToken_PostgreSQLCodeBindingsAndReplay(t *testing.T) {
	p := newTokenPG(t)
	p.code(t, "fixture-code")
	bad := codeForm()
	bad.Set("redirect_uri", "http://localhost:1234/cb")
	tokenJSON(t, requestToken(p.handler, bad), 400, "invalid_grant")
	var consumed bool
	if err := p.pool.QueryRow(t.Context(), `SELECT consumed_at IS NOT NULL FROM oauth_code WHERE code_hash=$1`, tokenDigest("fixture-code")).Scan(&consumed); err != nil || consumed {
		t.Fatalf("invalid request consumed code: %v", err)
	}
	got := tokenJSON(t, requestToken(p.handler, codeForm()), 200, "")
	token, err := store.New(p.pool).GetToken(t.Context(), tokenDigest(got["refresh_token"].(string)))
	if err != nil || token.AuthEpoch != p.account.AuthEpoch || token.Resource != tokenTestBase+"/mcp" {
		t.Fatal("incorrect persisted token")
	}
	p.code(t, "pending-code")
	tokenJSON(t, requestToken(p.handler, codeForm()), 400, "invalid_grant")
	p.assertRevoked(t)
}

func TestToken_PostgreSQLRefreshConcurrentReuse(t *testing.T) {
	p := newTokenPG(t)
	first := p.exchange(t, "first")
	p.exchange(t, "second")
	p.code(t, "pending")
	raw := first["refresh_token"].(string)
	before, err := store.New(p.pool).GetToken(t.Context(), tokenDigest(raw))
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	responses := make(chan int, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Go(func() { <-start; responses <- requestToken(p.handler, refreshForm(raw)).Code })
	}
	close(start)
	wg.Wait()
	close(responses)
	success, invalid := 0, 0
	for status := range responses {
		switch status {
		case 200:
			success++
		case 400:
			invalid++
		default:
			t.Fatalf("unexpected status %d", status)
		}
	}
	if success != 1 || invalid != 1 {
		t.Fatalf("success %d invalid %d", success, invalid)
	}
	p.assertRevoked(t)
	var count int
	err = p.pool.QueryRow(t.Context(), `SELECT count(*) FROM oauth_token WHERE family_id=$1 AND family_expires_at<>$2`, before.FamilyID, before.FamilyExpiresAt).Scan(&count)
	if err != nil || count != 0 {
		t.Fatalf("family expiry changed: %v", err)
	}
}

func TestToken_PostgreSQLFamilyCap(t *testing.T) {
	p := newTokenPG(t)
	first := p.exchange(t, "first")
	old, err := store.New(p.pool).GetToken(t.Context(), tokenDigest(first["refresh_token"].(string)))
	if err != nil {
		t.Fatal(err)
	}
	// Make the oldest family independent of transaction timestamp precision.
	if _, err = p.pool.Exec(t.Context(), `UPDATE oauth_token SET created_at=now()-interval '1 day' WHERE family_id=$1`, old.FamilyID); err != nil {
		t.Fatal(err)
	}
	// Rotation must not make an old family young again.
	tokenJSON(t, requestToken(p.handler, refreshForm(first["refresh_token"].(string))), 200, "")
	for i := range 10 {
		p.exchange(t, fmt.Sprintf("code-%d", i))
	}
	var liveFamilies, liveOld int
	err = p.pool.QueryRow(t.Context(), `SELECT count(DISTINCT family_id),count(*) FILTER(WHERE family_id=$2)
 FROM oauth_token WHERE grant_id=$1 AND revoked_at IS NULL`, p.grant.ID, old.FamilyID).Scan(&liveFamilies, &liveOld)
	if err != nil || liveFamilies != 10 || liveOld != 0 {
		t.Fatalf("live families %d, oldest tokens %d, error %v", liveFamilies, liveOld, err)
	}
}

func TestToken_PostgreSQLBlockedAndEpoch(t *testing.T) {
	for _, grant := range []string{"code", "refresh"} {
		for _, change := range []string{"blocked_at=now()", "auth_epoch=auth_epoch+1"} {
			t.Run(grant+change, func(t *testing.T) {
				p := newTokenPG(t)
				p.code(t, "fixture-code")
				form := codeForm()
				if grant == "refresh" {
					got := tokenJSON(t, requestToken(p.handler, form), 200, "")
					form = refreshForm(got["refresh_token"].(string))
				}
				if _, err := p.pool.Exec(t.Context(), `UPDATE account SET `+change+` WHERE id=$1`, p.account.ID); err != nil {
					t.Fatal(err)
				}
				tokenJSON(t, requestToken(p.handler, form), 400, "invalid_grant")
			})
		}
	}
}

func TestToken_PostgreSQLGrantRestrictions(t *testing.T) {
	t.Run("code grant denied", func(t *testing.T) {
		p := newTokenPG(t)
		p.code(t, "fixture-code")
		if _, err := p.pool.Exec(t.Context(), `UPDATE oauth_client SET grant_types=ARRAY['refresh_token'] WHERE client_id='client'`); err != nil {
			t.Fatal(err)
		}
		tokenJSON(t, requestToken(p.handler, codeForm()), 400, "unauthorized_client")
		var consumed bool
		if err := p.pool.QueryRow(t.Context(), `SELECT consumed_at IS NOT NULL FROM oauth_code WHERE code_hash=$1`, tokenDigest("fixture-code")).Scan(&consumed); err != nil || consumed {
			t.Fatalf("code consumed %v: %v", consumed, err)
		}
	})
	t.Run("refresh grant denied", func(t *testing.T) {
		p := newTokenPG(t)
		got := p.exchange(t, "fixture-code")
		raw := got["refresh_token"].(string)
		if _, err := p.pool.Exec(t.Context(), `UPDATE oauth_client SET grant_types=ARRAY['authorization_code'] WHERE client_id='client'`); err != nil {
			t.Fatal(err)
		}
		tokenJSON(t, requestToken(p.handler, refreshForm(raw)), 400, "unauthorized_client")
		row, err := store.New(p.pool).GetToken(t.Context(), tokenDigest(raw))
		if err != nil || row.SupersededAt != nil {
			t.Fatalf("refresh consumed: %v", err)
		}
	})
	t.Run("access only", func(t *testing.T) {
		p := newTokenPG(t)
		p.code(t, "fixture-code")
		if _, err := p.pool.Exec(t.Context(), `UPDATE oauth_client SET grant_types=ARRAY['authorization_code'] WHERE client_id='client'`); err != nil {
			t.Fatal(err)
		}
		before := time.Now()
		got := tokenJSON(t, requestToken(p.handler, codeForm()), 200, "")
		after := time.Now()
		if _, ok := got["refresh_token"]; ok {
			t.Fatal("refresh token returned")
		}
		row, err := store.New(p.pool).GetToken(t.Context(), tokenDigest(got["access_token"].(string)))
		if err != nil || !row.ExpiresAt.Equal(row.FamilyExpiresAt) || row.ExpiresAt.Before(before.Add(time.Hour-time.Millisecond)) || row.ExpiresAt.After(after.Add(time.Hour+time.Millisecond)) {
			t.Fatalf("access family expiry: %v", err)
		}
		var count int
		if err = p.pool.QueryRow(t.Context(), `SELECT count(*) FROM oauth_token WHERE family_id=$1`, row.FamilyID).Scan(&count); err != nil || count != 1 {
			t.Fatalf("family has %d tokens: %v", count, err)
		}
	})
}
