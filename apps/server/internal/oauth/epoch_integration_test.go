//go:build integration

package oauth

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/store"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type epochPool struct {
	*pgxpool.Pool
	lock chan struct{}
	once sync.Once
}

func (p *epochPool) Begin(ctx context.Context) (pgx.Tx, error) {
	tx, err := p.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return nil, err
	}
	return epochTx{Tx: tx, p: p}, nil
}

type epochTx struct {
	pgx.Tx
	p *epochPool
}

func (tx epochTx) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	if strings.Contains(sql, "LockCredentialAccount") {
		tx.p.once.Do(func() { close(tx.p.lock) })
	}
	return tx.Tx.QueryRow(ctx, sql, args...)
}

func TestOAuth_BumpEpochRacesIssuers(t *testing.T) {
	for _, kind := range []string{"consent", "code", "refresh"} {
		t.Run(kind, func(t *testing.T) {
			var pool *pgxpool.Pool
			var accountID int64
			var issue func(*epochPool) int
			if kind == "consent" {
				pool = consentPostgres(t)
				accountID = 7
				path, cookie := postgresPending(t, pool, "fixture")
				issue = func(db *epochPool) int {
					w := requestConsentWithDB(db, path, cookie)
					return w
				}
			} else {
				p := newTokenPG(t)
				pool = p.pool
				accountID = p.account.ID
				form := codeForm()
				if kind == "code" {
					p.code(t, "fixture-code")
				} else {
					form = refreshForm(p.exchange(t, "fixture-code")["refresh_token"].(string))
				}
				issue = func(db *epochPool) int { return requestToken(NewTokenHandler(db, tokenTestBase, nil), form).Code }
			}
			bump, err := pool.Begin(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = bump.Rollback(t.Context()) }()
			if _, err := store.BumpEpoch(t.Context(), bump, accountID); err != nil {
				t.Fatal(err)
			}
			db := &epochPool{Pool: pool, lock: make(chan struct{})}
			result := make(chan int, 1)
			go func() { result <- issue(db) }()
			select {
			case <-db.lock:
			case <-time.After(2 * time.Second):
				t.Fatal("issuer did not lock account")
			}
			if err := bump.Commit(t.Context()); err != nil {
				t.Fatal(err)
			}
			if status := <-result; status != 400 {
				t.Fatalf("stale issuer status=%d", status)
			}
			for _, table := range []string{"oauth_code", "oauth_token"} {
				var live int
				if err := pool.QueryRow(t.Context(), fmt.Sprintf(`SELECT count(*) FROM %s WHERE account_id=$1 AND auth_epoch < (SELECT auth_epoch FROM account WHERE id=$1) AND revoked_at IS NULL`, table), accountID).Scan(&live); err != nil || live != 0 {
					t.Fatalf("%s live old=%d err=%v", table, live, err)
				}
			}
		})
	}
}

func TestOAuth_StaleEpochWithoutRevocationRefuses(t *testing.T) {
	for _, kind := range []string{"consent", "code", "refresh"} {
		t.Run(kind, func(t *testing.T) {
			if kind == "consent" {
				pool := consentPostgres(t)
				path, cookie := postgresPending(t, pool, "fixture")
				if _, err := pool.Exec(t.Context(), `UPDATE account SET auth_epoch=auth_epoch+1 WHERE id=7`); err != nil {
					t.Fatal(err)
				}
				if status := postgresConsent(pool, path, cookie); status != 400 {
					t.Fatal(status)
				}
				return
			}
			p := newTokenPG(t)
			form := codeForm()
			if kind == "code" {
				p.code(t, "fixture-code")
			} else {
				form = refreshForm(p.exchange(t, "fixture-code")["refresh_token"].(string))
			}
			if _, err := p.pool.Exec(t.Context(), `UPDATE account SET auth_epoch=auth_epoch+1 WHERE id=$1`, p.account.ID); err != nil {
				t.Fatal(err)
			}
			tokenJSON(t, requestToken(p.handler, form), 400, "invalid_grant")
		})
	}
}

func requestConsentWithDB(db *epochPool, path string, cookie *http.Cookie) int {
	w := httptest.NewRecorder()
	NewConsentHandler(consentBase, db, consentSessionManager()).ServeHTTP(w, consentRequest(path, "POST", cookie, true, formFor(path, "approve")))
	return w.Code
}
