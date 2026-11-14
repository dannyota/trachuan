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

	"danny.vn/trachuan/apps/server/internal/accountflow"
	"danny.vn/trachuan/apps/server/internal/seal"
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

func TestOAuth_ResetRacesIssuers(t *testing.T) {
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
			stagePasswordReset(t, pool, bump, accountID)
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

type stagedResetDB struct{ tx pgx.Tx }
type stagedResetTx struct{ pgx.Tx }

func (db stagedResetDB) BeginTx(context.Context, pgx.TxOptions) (pgx.Tx, error) {
	return stagedResetTx{db.tx}, nil
}
func (stagedResetTx) Commit(context.Context) error   { return nil }
func (stagedResetTx) Rollback(context.Context) error { return nil }
func stagePasswordReset(t *testing.T, pool *pgxpool.Pool, tx pgx.Tx, id int64) {
	t.Helper()
	ctx := t.Context()
	if _, err := pool.Exec(ctx, `INSERT INTO password_credential(account_id,hash) VALUES ($1,'old') ON CONFLICT DO NOTHING`, id); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO email_link(purpose,token_hash,email,account_id,auth_epoch,expires_at) SELECT 'reset',repeat('9',64),email,id,auth_epoch,now()+interval '1 hour' FROM account WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	ring, err := seal.Parse([]byte(strings.Repeat("A", 43)))
	if err != nil {
		t.Fatal(err)
	}
	service := accountflow.Service{DB: stagedResetDB{tx}, Ring: ring}
	if got, err := service.CompleteReset(ctx, accountflow.Completion{TokenHash: strings.Repeat("9", 64), Hash: "new"}); err != nil || got != id {
		t.Fatal("stage reset", err)
	}
}
