//go:build integration

package oauth

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"danny.vn/trachuan/apps/server/internal/migrate"
	"danny.vn/trachuan/apps/server/internal/store"
	"danny.vn/trachuan/internal/testpg"
	"github.com/jackc/pgx/v5/pgxpool"
)

func consentPostgres(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := testpg.NewDatabase(t)
	if err := migrate.Up(t.Context(), dsn); err != nil {
		t.Fatal(err)
	}
	db, err := pgxpool.New(t.Context(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	if _, err = db.Exec(t.Context(), `INSERT INTO account(id,google_sub,email,auth_epoch) OVERRIDING SYSTEM VALUE VALUES(7,'consent-fixture','fixture@example.invalid',3)`); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(t.Context(), `INSERT INTO web_session(id,token_hash,account_id,csrf_secret,auth_epoch,expires_at) OVERRIDING SYSTEM VALUE VALUES(1,repeat('a',64),7,'session-secret-fixture',3,now()+interval '1 hour')`); err != nil {
		t.Fatal(err)
	}

	if _, err = store.New(db).CreateClient(t.Context(), store.CreateClientParams{ClientID: "fixture", Kind: "dcr", Name: "Fixture", RedirectUris: []string{"http://localhost/cb"}}); err != nil {
		t.Fatal(err)
	}
	return db
}
func postgresPending(t *testing.T, db *pgxpool.Pool, id string) (string, *http.Cookie) {
	t.Helper()
	v := authorizeValues()
	v.Set("client_id", id)
	w := httptest.NewRecorder()
	NewAuthorizeHandler(consentBase, db, NewClients(db, nil, nil).Resolve).ServeHTTP(w, httptest.NewRequest("GET", "/oauth/authorize?"+v.Encode(), nil))
	if w.Code != 303 {
		t.Fatalf("authorize status %d", w.Code)
	}
	return w.Header().Get("Location"), w.Result().Cookies()[0]
}
func postgresConsent(db *pgxpool.Pool, path string, cookie *http.Cookie) int {
	w := httptest.NewRecorder()
	NewConsentHandler(consentBase, db, consentSessionManager()).ServeHTTP(w, consentRequest(path, "POST", cookie, true, formFor(path, "approve")))
	return w.Code
}
func TestConsent_ClaimOnceUnderRace_Postgres(t *testing.T) {
	db := consentPostgres(t)
	path, cookie := postgresPending(t, db, "fixture")
	results := make(chan int, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Go(func() { results <- postgresConsent(db, path, cookie) })
	}
	wg.Wait()
	close(results)
	success := 0
	for status := range results {
		if status == 303 {
			success++
		} else if status != 400 {
			t.Fatalf("status %d", status)
		}
	}
	var codes int
	if err := db.QueryRow(t.Context(), `SELECT count(*) FROM oauth_code`).Scan(&codes); err != nil {
		t.Fatal(err)
	}
	if success != 1 || codes != 1 {
		t.Fatalf("successes %d codes %d", success, codes)
	}
	var accountID int64
	if err := db.QueryRow(t.Context(), `SELECT account_id FROM oauth_request`).Scan(&accountID); err != nil || accountID != 7 {
		t.Fatalf("claim account %d error %v", accountID, err)
	}
}
func seedConsentGrants(t *testing.T, db *pgxpool.Pool, count int) {
	t.Helper()
	if _, err := db.Exec(t.Context(), `INSERT INTO oauth_client(client_id,kind,name,redirect_uris) SELECT 'other-'||i,'dcr','Fixture',ARRAY['http://localhost/cb'] FROM generate_series(1,$1::int) i`, count); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(t.Context(), `INSERT INTO oauth_grant(account_id,client_id,scope,resource) SELECT 7,client_id,'mcp:read',$1 FROM oauth_client WHERE client_id LIKE 'other-%'`, consentBase+"/mcp"); err != nil {
		t.Fatal(err)
	}
}
func TestConsent_GrantCap_Postgres(t *testing.T) {
	for _, reuse := range []bool{false, true} {
		t.Run(fmt.Sprint(reuse), func(t *testing.T) {
			db := consentPostgres(t)
			count := 20
			if reuse {
				count = 19
			}
			seedConsentGrants(t, db, count)
			if reuse {
				if _, err := store.New(db).UpsertGrant(t.Context(), store.UpsertGrantParams{AccountID: 7, ClientID: "fixture", Scope: "mcp:read", Resource: consentBase + "/mcp"}); err != nil {
					t.Fatal(err)
				}
			}
			path, cookie := postgresPending(t, db, "fixture")
			status := postgresConsent(db, path, cookie)
			want := 400
			if reuse {
				want = 303
			}
			if status != want {
				t.Fatalf("status %d want %d", status, want)
			}
			if live, err := store.New(db).CountLiveGrants(t.Context(), 7); err != nil || live != 20 {
				t.Fatalf("live %d error %v", live, err)
			}
			if !reuse {
				var claimed bool
				if err := db.QueryRow(t.Context(), `SELECT claimed_at IS NOT NULL FROM oauth_request`).Scan(&claimed); err != nil || claimed {
					t.Fatalf("failed cap claimed request: %v", err)
				}
			}
		})
	}
}
func TestConsent_GrantCapConcurrent_Postgres(t *testing.T) {
	db := consentPostgres(t)
	seedConsentGrants(t, db, 19)
	if _, err := store.New(db).CreateClient(t.Context(), store.CreateClientParams{ClientID: "second", Kind: "dcr", Name: "Fixture", RedirectUris: []string{"http://localhost/cb"}}); err != nil {
		t.Fatal(err)
	}
	first, c1 := postgresPending(t, db, "fixture")
	second, c2 := postgresPending(t, db, "second")
	results := make(chan int, 2)
	var wg sync.WaitGroup
	wg.Go(func() { results <- postgresConsent(db, first, c1) })
	wg.Go(func() { results <- postgresConsent(db, second, c2) })
	wg.Wait()
	close(results)
	success := 0
	for status := range results {
		if status == 303 {
			success++
		} else if status != 400 {
			t.Fatalf("status %d", status)
		}
	}
	if live, err := store.New(db).CountLiveGrants(t.Context(), 7); err != nil || live != 20 || success != 1 {
		t.Fatalf("live %d successes %d error %v", live, success, err)
	}
}
func TestAuthorize_PendingCapConcurrent_Postgres(t *testing.T) {
	db := consentPostgres(t)
	if _, err := db.Exec(t.Context(), `INSERT INTO oauth_request(handle_hash,browser_hash,client_id,redirect_uri,state,code_challenge,resource,scope,expires_at) SELECT encode(sha256(i::text::bytea),'hex'),repeat('a',64),'fixture','http://localhost/cb','','challenge',$1,'mcp:read',now()+interval '10 minutes' FROM generate_series(1,9999) i`, consentBase+"/mcp"); err != nil {
		t.Fatal(err)
	}
	results := make(chan int, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Go(func() {
			w := httptest.NewRecorder()
			NewAuthorizeHandler(consentBase, db, NewClients(db, nil, nil).Resolve).ServeHTTP(w, httptest.NewRequest("GET", "/oauth/authorize?"+authorizeValues().Encode(), nil))
			results <- w.Code
		})
	}
	wg.Wait()
	close(results)
	success := 0
	for status := range results {
		if status == 303 {
			success++
		} else if status != 503 {
			t.Fatalf("status %d", status)
		}
	}
	if count, err := store.New(db).CountPendingRequests(t.Context()); err != nil || count != 10000 || success != 1 {
		t.Fatalf("pending %d successes %d error %v", count, success, err)
	}
}
func TestConsent_ReconsentPreservesIssuedEpoch_Postgres(t *testing.T) {
	db := consentPostgres(t)
	path, cookie := postgresPending(t, db, "fixture")
	if status := postgresConsent(db, path, cookie); status != 303 {
		t.Fatalf("first status %d", status)
	}
	if _, err := db.Exec(t.Context(), `UPDATE account SET auth_epoch=4 WHERE id=7`); err != nil {
		t.Fatal(err)
	}
	// A fresh login at the new epoch may reuse the grant without changing old codes.
	if _, err := db.Exec(t.Context(), `UPDATE web_session SET auth_epoch=4 WHERE id=1`); err != nil {
		t.Fatal(err)
	}

	path, cookie = postgresPending(t, db, "fixture")
	if status := postgresConsent(db, path, cookie); status != 303 {
		t.Fatalf("second status %d", status)
	}
	var old, newer int
	if err := db.QueryRow(context.Background(), `SELECT count(*) FILTER(WHERE auth_epoch=3),count(*) FILTER(WHERE auth_epoch=4) FROM oauth_code`).Scan(&old, &newer); err != nil || old != 1 || newer != 1 {
		t.Fatalf("epochs %d %d error %v", old, newer, err)
	}
	if grants, err := store.New(db).CountLiveGrants(t.Context(), 7); err != nil || grants != 1 {
		t.Fatalf("grants %d error %v", grants, err)
	}
}

func TestConsent_StaleCachedSession_Postgres(t *testing.T) {
	for _, change := range []string{"epoch", "revocation", "expiry"} {
		t.Run(change, func(t *testing.T) {
			db := consentPostgres(t)
			path, cookie := postgresPending(t, db, "fixture")
			sql := `UPDATE account SET auth_epoch=4 WHERE id=7`
			if change == "revocation" {
				sql = `UPDATE web_session SET revoked_at=now() WHERE id=1`
			}
			if change == "expiry" {
				sql = `UPDATE web_session SET expires_at=now()-interval '1 second' WHERE id=1`
			}
			if _, err := db.Exec(t.Context(), sql); err != nil {
				t.Fatal(err)
			}
			if status := postgresConsent(db, path, cookie); status != 400 {
				t.Fatalf("stale session status %d", status)
			}
			var codes int
			if err := db.QueryRow(t.Context(), `SELECT count(*) FROM oauth_code`).Scan(&codes); err != nil || codes != 0 {
				t.Fatalf("codes %d error %v", codes, err)
			}
		})
	}
}
