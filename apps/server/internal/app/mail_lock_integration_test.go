//go:build integration

package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/accountflow"
	"danny.vn/trachuan/apps/server/internal/passwordauth"
	"danny.vn/trachuan/apps/server/internal/seal"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type mailLockDB struct {
	pool         *pgxpool.Pool
	reset        bool
	held, resume chan struct{}
	signup       chan uint32
}
type mailLockTx struct {
	pgx.Tx
	db     mailLockDB
	paused bool
}
type mailLockRow struct {
	pgx.Row
	tx  *mailLockTx
	ctx context.Context
}

func (db mailLockDB) BeginTx(ctx context.Context, opts pgx.TxOptions) (pgx.Tx, error) {
	tx, err := db.pool.BeginTx(ctx, opts)
	if err != nil {
		return nil, err
	}
	return &mailLockTx{Tx: tx, db: db}, nil
}
func (tx *mailLockTx) pause(ctx context.Context) error {
	if tx.paused {
		return nil
	}
	tx.paused = true
	close(tx.db.held)
	select {
	case <-tx.db.resume:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (tx *mailLockTx) Exec(ctx context.Context, query string, args ...any) (pgconn.CommandTag, error) {
	admission := strings.Contains(query, "pg_advisory_xact_lock(739215,1)")
	if admission && !tx.db.reset {
		tx.db.signup <- tx.Conn().PgConn().PID()
	}
	tag, err := tx.Tx.Exec(ctx, query, args...)
	if err == nil && admission && tx.db.reset {
		err = tx.pause(ctx)
	}
	return tag, err
}
func (tx *mailLockTx) QueryRow(ctx context.Context, query string, args ...any) pgx.Row {
	row := tx.Tx.QueryRow(ctx, query, args...)
	if tx.db.reset && strings.Contains(query, "LockCredentialAccount") {
		return mailLockRow{row, tx, ctx}
	}
	return row
}
func (r mailLockRow) Scan(dest ...any) error {
	if err := r.Row.Scan(dest...); err != nil {
		return err
	}
	return r.tx.pause(r.ctx)
}
func TestAccounts_ConcurrentSignupAndResetFinishUniformly(t *testing.T) {
	_, pool, _, id := boundsFixture(t)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	if _, err := pool.Exec(ctx, `INSERT INTO password_credential(account_id,hash) VALUES ($1,'synthetic')`, id); err != nil {
		t.Fatal(err)
	}
	ring, err := seal.Parse([]byte(strings.Repeat("A", 43)))
	if err != nil {
		t.Fatal(err)
	}
	held, resume := make(chan struct{}), make(chan struct{})
	signup := make(chan uint32, 1)
	var release sync.Once
	defer release.Do(func() { close(resume) })
	results := make(chan *httptest.ResponseRecorder, 2)
	request := func(reset bool) {
		db := mailLockDB{pool: pool, reset: reset, held: held, resume: resume, signup: signup}
		h := passwordauth.Handler{Flows: &accountflow.Service{DB: db, Ring: ring}, PublicURL: "https://example.test"}
		path, handler := "signup", http.HandlerFunc(h.Signup)
		if reset {
			path, handler = "reset", h.Reset
		}
		r := httptest.NewRequest("POST", "/api/v1/auth/"+path, strings.NewReader(`{"email":"test@example.invalid"}`)).WithContext(ctx)
		r.Header.Set("Origin", h.PublicURL)
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		results <- w
	}
	go request(true)
	select {
	case <-held:
	case <-ctx.Done():
		t.Fatal("reset did not acquire its first lock")
	}
	go request(false)
	var pid uint32
	select {
	case pid = <-signup:
	case <-ctx.Done():
		t.Fatal("signup did not request admission")
	}
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		var waitingOrHeld bool
		err = pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_locks WHERE locktype='advisory' AND pid=$1 AND classid=739215::oid AND objid=1::oid)`, pid).Scan(&waitingOrHeld)
		if err != nil {
			t.Fatal(err)
		}
		if waitingOrHeld {
			break
		}
		select {
		case <-ticker.C:
		case <-ctx.Done():
			t.Fatal("signup admission did not reach PostgreSQL")
		}
	}
	release.Do(func() { close(resume) })
	for range 2 {
		select {
		case w := <-results:
			if w.Code != 202 || w.Body.String() != "{}\n" {
				t.Fatalf("concurrent request status=%d body=%q", w.Code, w.Body.String())
			}
		case <-ctx.Done():
			t.Fatal("concurrent mail requests did not finish")
		}
	}
}
