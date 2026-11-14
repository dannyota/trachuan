//go:build integration

package accountflow

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/factor"
	"danny.vn/trachuan/apps/server/internal/store"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func pendingGoogle(t *testing.T, q *store.Queries) store.CreateSessionParams {
	t.Helper()
	p := factorSessionParams()
	_, err := q.CreateGoogleSession(t.Context(), store.GoogleSessionParams{Identity: store.GoogleIdentity{Sub: "factor"}, Session: p})
	var required *store.PendingRequired
	if !errors.As(err, &required) {
		t.Fatal("primary did not require factor", err)
	}
	return p
}
func TestFactor_ConcurrentRecoveryCodeRedemption(t *testing.T) {
	s, db, a := factorFixture(t)
	a, codes, _ := enroll(t, s, a)
	pending := []store.CreateSessionParams{pendingGoogle(t, store.New(db)), pendingGoogle(t, store.New(db))}
	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for _, p := range pending {
		wg.Go(func() {
			<-start
			_, _, err := s.CompleteFactor(t.Context(), FactorCompletion{TokenHash: p.TokenHash, CSRF: p.CsrfSecret, RecoveryCode: codes[0], UseRecovery: true, Session: factorSessionParams()})
			results <- err
		})
	}
	close(start)
	wg.Wait()
	close(results)
	accepted, refused := 0, 0
	for err := range results {
		if err == nil {
			accepted++
		} else if errors.Is(err, factor.ErrFailed) {
			refused++
		} else {
			t.Fatal(err)
		}
	}
	if accepted != 1 || refused != 1 {
		t.Fatal("recovery code was not single use")
	}
	var used, attempts, consumed int
	if err := db.QueryRow(t.Context(), `SELECT (SELECT count(*) FROM recovery_code WHERE account_id=$1 AND used_at IS NOT NULL),(SELECT count(*) FROM recovery_attempt WHERE account_id=$1),(SELECT count(*) FROM pending_auth WHERE account_id=$1 AND consumed_at IS NOT NULL)`, a.AccountID).Scan(&used, &attempts, &consumed); err != nil || used != 1 || attempts != 1 || consumed != 1 {
		t.Fatal("recovery redemption side effects", err)
	}
}

type factorRaceDB struct {
	*pgxpool.Pool
	locked, committing, release chan struct{}
	once                        sync.Once
}

func (db *factorRaceDB) BeginTx(ctx context.Context, opts pgx.TxOptions) (pgx.Tx, error) {
	tx, err := db.Pool.BeginTx(ctx, opts)
	if err != nil {
		return nil, err
	}
	return factorRaceTx{Tx: tx, db: db}, nil
}

type factorRaceTx struct {
	pgx.Tx
	db *factorRaceDB
}

func (tx factorRaceTx) QueryRow(ctx context.Context, query string, args ...any) pgx.Row {
	if tx.db.locked != nil && strings.Contains(query, "LockCredentialAccount") {
		tx.db.once.Do(func() { close(tx.db.locked) })
	}
	return tx.Tx.QueryRow(ctx, query, args...)
}
func (tx factorRaceTx) Commit(ctx context.Context) error {
	if tx.db.committing != nil {
		close(tx.db.committing)
		select {
		case <-tx.db.release:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return tx.Tx.Commit(ctx)
}

type heldFactorDB struct{ tx pgx.Tx }

func (db heldFactorDB) BeginTx(context.Context, pgx.TxOptions) (pgx.Tx, error) {
	return heldFactorTx{db.tx}, nil
}

type heldFactorTx struct{ pgx.Tx }

func (heldFactorTx) Commit(context.Context) error   { return nil }
func (heldFactorTx) Rollback(context.Context) error { return nil }

func waitFactorRace(t *testing.T, signal <-chan struct{}) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(2 * time.Second):
		t.Fatal("factor race did not reach transaction boundary")
	}
}
func TestFactor_CompletionRacingResetFencesOldEpoch(t *testing.T) {
	for _, first := range []string{"reset", "factor"} {
		t.Run(first+" first", func(t *testing.T) {
			s, db, a := factorFixture(t)
			a, codes, _ := enroll(t, s, a)
			p := pendingGoogle(t, store.New(db))
			if _, err := db.Exec(t.Context(), `INSERT INTO password_credential(account_id,hash) VALUES($1,'old')`, a.AccountID); err != nil {
				t.Fatal(err)
			}
			resetHash := factorSessionParams().TokenHash
			if _, err := db.Exec(t.Context(), `INSERT INTO email_link(purpose,token_hash,email,account_id,auth_epoch,expires_at) SELECT 'reset',$2,email,id,auth_epoch,now()+interval '1 hour' FROM account WHERE id=$1`, a.AccountID, resetHash); err != nil {
				t.Fatal(err)
			}
			completion := FactorCompletion{TokenHash: p.TokenHash, CSRF: p.CsrfSecret, RecoveryCode: codes[0], UseRecovery: true, Session: factorSessionParams()}
			reset := Completion{TokenHash: resetHash, Hash: "new"}
			type outcome struct {
				row store.CreateSessionRow
				err error
			}
			issued := make(chan outcome, 1)
			if first == "reset" {
				tx, err := db.Begin(t.Context())
				if err != nil {
					t.Fatal(err)
				}
				defer func() { _ = tx.Rollback(t.Context()) }()
				staged := &Service{DB: heldFactorDB{tx}, Ring: s.Ring}
				if _, err = staged.CompleteReset(t.Context(), reset); err != nil {
					t.Fatal(err)
				}
				observed := &factorRaceDB{Pool: db, locked: make(chan struct{})}
				completing := &Service{DB: observed, Ring: s.Ring}
				go func() {
					row, _, issueErr := completing.CompleteFactor(t.Context(), completion)
					issued <- outcome{row, issueErr}
				}()
				waitFactorRace(t, observed.locked)
				if err = tx.Commit(t.Context()); err != nil {
					t.Fatal(err)
				}
				result := <-issued
				if !errors.Is(result.err, factor.ErrFailed) || result.row.ID != 0 {
					t.Fatal("factor completion issued after reset", result.err)
				}
			} else {
				release := make(chan struct{})
				var once sync.Once
				unblock := func() { once.Do(func() { close(release) }) }
				defer unblock()
				observed := &factorRaceDB{Pool: db, committing: make(chan struct{}), release: release}
				completing := &Service{DB: observed, Ring: s.Ring}
				go func() { row, _, err := completing.CompleteFactor(t.Context(), completion); issued <- outcome{row, err} }()
				waitFactorRace(t, observed.committing)
				waiting := &factorRaceDB{Pool: db, locked: make(chan struct{})}
				resetting := &Service{DB: waiting, Ring: s.Ring}
				changed := make(chan error, 1)
				go func() { _, err := resetting.CompleteReset(t.Context(), reset); changed <- err }()
				waitFactorRace(t, waiting.locked)
				unblock()
				result := <-issued
				if result.err != nil || result.row.AuthEpoch != a.AuthEpoch {
					t.Fatal("factor did not complete before reset", result.err)
				}
				if err := <-changed; err != nil {
					t.Fatal(err)
				}
				var revoked bool
				if err := db.QueryRow(t.Context(), `SELECT revoked_at IS NOT NULL FROM web_session WHERE id=$1`, result.row.ID).Scan(&revoked); err != nil || !revoked {
					t.Fatal("reset kept factor-issued session", err)
				}
			}
			var epoch, oldLive int32
			var hash string
			if err := db.QueryRow(t.Context(), `SELECT a.auth_epoch,p.hash,(SELECT count(*)::integer FROM web_session s WHERE s.account_id=a.id AND s.auth_epoch<a.auth_epoch AND s.revoked_at IS NULL) FROM account a JOIN password_credential p ON p.account_id=a.id WHERE a.id=$1`, a.AccountID).Scan(&epoch, &hash, &oldLive); err != nil || epoch != a.AuthEpoch+1 || hash != "new" || oldLive != 0 {
				t.Fatal("reset race left old authority", err)
			}
		})
	}
}
