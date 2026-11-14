//go:build integration

package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/accountflow"
	"danny.vn/trachuan/apps/server/internal/credential"
	"danny.vn/trachuan/apps/server/internal/seal"
	"danny.vn/trachuan/apps/server/internal/store"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type issuerPool struct {
	*pgxpool.Pool
	lock chan struct{}
	once sync.Once
}

func (p *issuerPool) Begin(ctx context.Context) (pgx.Tx, error) {
	tx, err := p.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return nil, err
	}
	return issuerTx{Tx: tx, p: p}, nil
}
func (p *issuerPool) BeginTx(ctx context.Context, opts pgx.TxOptions) (pgx.Tx, error) {
	tx, err := p.Pool.BeginTx(ctx, opts)
	if err != nil {
		return nil, err
	}
	return issuerTx{Tx: tx, p: p}, nil
}

type issuerTx struct {
	pgx.Tx
	p *issuerPool
}

func (tx issuerTx) QueryRow(ctx context.Context, query string, args ...any) pgx.Row {
	if strings.Contains(query, "LockCredentialAccount") {
		tx.p.once.Do(func() { close(tx.p.lock) })
	}
	return tx.Tx.QueryRow(ctx, query, args...)
}

func TestIssuers_ResetRace(t *testing.T) {
	for _, kind := range []string{"session", "sliding", "web token", "admin token", "Google", "password", "reauth", "change password"} {
		t.Run(kind, func(t *testing.T) {
			ctx, pool, _, id := boundsFixture(t)
			observed := &issuerPool{Pool: pool, lock: make(chan struct{})}
			s := credentialStore{Queries: store.New(observed), db: observed}
			now := time.Now()
			session, err := store.New(pool).CreateSession(ctx, store.CreateSessionParams{TokenHash: strings.Repeat("a", 64), AccountID: id, CsrfSecret: "synthetic", ExpiresAt: now.Add(time.Hour)})
			if err != nil {
				t.Fatal(err)
			}
			if _, err = pool.Exec(ctx, `UPDATE web_session SET last_seen_at=now()-interval '2 hours' WHERE id=$1`, session.ID); err != nil {
				t.Fatal(err)
			}
			bump, err := pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = bump.Rollback(ctx) }()
			stagePasswordReset(t, pool, bump, id)
			oldHash, oldVersion := "old", int64(1)
			verified := store.PasswordSnapshot{AccountID: id, Email: "test@example.invalid", Hash: &oldHash, Version: &oldVersion}
			ring, err := seal.Parse([]byte(strings.Repeat("A", 43)))
			if err != nil {
				t.Fatal(err)
			}
			changes := accountflow.Service{DB: observed, Ring: ring}
			result := make(chan error, 1)
			go func() {
				var issueErr error
				switch kind {
				case "change password":
					_, issueErr = changes.ChangePassword(ctx, accountflow.Completion{Acting: accountflow.Acting{AccountID: id, SessionID: session.ID}, Verified: verified, Hash: "new", Session: store.CreateSessionParams{TokenHash: strings.Repeat("8", 64), CsrfSecret: "synthetic", ExpiresAt: now.Add(time.Hour)}})
				case "password":
					_, issueErr = s.CreatePasswordSession(ctx, store.PasswordSessionParams{Verified: verified, Session: store.CreateSessionParams{TokenHash: strings.Repeat("f", 64), CsrfSecret: "synthetic", ExpiresAt: now.Add(time.Hour)}})
				case "reauth":
					issueErr = s.ReauthPassword(ctx, verified, session.ID)
				case "session":
					_, issueErr = s.CreateSession(ctx, store.CreateSessionParams{AccountID: id, TokenHash: strings.Repeat("b", 64), CsrfSecret: "synthetic", ExpiresAt: now.Add(time.Hour)})
				case "sliding":
					_, issueErr = s.TouchSession(ctx, session.TokenHash)
				case "web token":
					_, issueErr = s.CreateWebPersonalToken(ctx, store.WebPersonalTokenParams{Token: store.CreatePersonalTokenParams{AccountID: id, TokenHash: strings.Repeat("c", 64), Name: "synthetic", ExpiresAt: now.Add(time.Hour)}, SessionID: session.ID})
				case "admin token":
					issueErr = runAdmin(ctx, []string{"token", "create", "--account", fmt.Sprint(id), "--name", "synthetic"}, s, io.Discard)
				case "Google":
					_, issueErr = s.CreateGoogleSession(ctx, store.GoogleSessionParams{Identity: store.GoogleIdentity{Sub: "bounds", Email: "changed@example.test"}, Session: store.CreateSessionParams{TokenHash: strings.Repeat("e", 64), CsrfSecret: "synthetic", ExpiresAt: now.Add(time.Hour)}})
				}
				result <- issueErr
			}()
			select {
			case <-observed.lock:
			case <-time.After(2 * time.Second):
				t.Fatal("issuer did not lock account")
			}
			if err = bump.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			err = <-result
			if kind == "Google" {
				if err != nil {
					t.Fatal(err)
				}
			} else if kind == "admin token" {
				if err == nil {
					t.Fatal("stale admin issued token")
				}
			} else if kind == "password" || kind == "reauth" {
				if !errors.Is(err, store.ErrLoginFailed) {
					t.Fatalf("stale password issuer: %v", err)
				}
			} else if !errors.Is(err, credential.ErrUnauthorized) {
				t.Fatalf("stale issuer error=%v", err)
			}
			var actingRevoked bool
			if err := pool.QueryRow(ctx, `SELECT revoked_at IS NOT NULL FROM web_session WHERE id=$1`, session.ID).Scan(&actingRevoked); err != nil || !actingRevoked {
				t.Fatal("acting session survived reset", err)
			}
			for _, table := range []string{"web_session", "personal_token"} {
				var live int
				if err := pool.QueryRow(ctx, fmt.Sprintf(`SELECT count(*) FROM %s WHERE account_id=$1 AND auth_epoch=0 AND revoked_at IS NULL`, table), id).Scan(&live); err != nil || live != 0 {
					t.Fatalf("%s old live=%d err=%v", table, live, err)
				}
			}
		})
	}
}

func TestAdmin_EnrolledCommandRefuses(t *testing.T) {
	ctx, pool, s, id := boundsFixture(t)
	if _, err := pool.Exec(ctx, `INSERT INTO totp_credential(account_id,sealed_secret,key_id) VALUES ($1,'synthetic','synthetic')`, id); err != nil {
		t.Fatal(err)
	}
	err := runAdmin(ctx, []string{"token", "create", "--account", fmt.Sprint(id), "--name", "synthetic"}, s, io.Discard)
	if !errors.Is(err, store.ErrFactorEnrolled) {
		t.Fatalf("operator refusal=%v", err)
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM personal_token`).Scan(&count); err != nil || count != 0 {
		t.Fatal("enrolled operator token issued")
	}
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
