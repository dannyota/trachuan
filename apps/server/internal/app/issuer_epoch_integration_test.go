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

	"danny.vn/trachuan/apps/server/internal/credential"
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

func TestIssuers_BumpEpochRace(t *testing.T) {
	for _, kind := range []string{"session", "sliding", "web token", "admin token", "Google"} {
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
			if _, err = store.BumpEpoch(ctx, bump, id); err != nil {
				t.Fatal(err)
			}
			result := make(chan error, 1)
			go func() {
				var issueErr error
				switch kind {
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
			} else if !errors.Is(err, credential.ErrUnauthorized) {
				t.Fatalf("stale issuer error=%v", err)
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
