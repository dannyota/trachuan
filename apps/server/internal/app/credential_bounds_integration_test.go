//go:build integration

package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/credential"
	"danny.vn/trachuan/apps/server/internal/migrate"
	"danny.vn/trachuan/apps/server/internal/store"
	"danny.vn/trachuan/apps/server/internal/websession"
	"danny.vn/trachuan/internal/testpg"
	"github.com/jackc/pgx/v5/pgxpool"
)

func boundsFixture(t *testing.T) (context.Context, *pgxpool.Pool, credentialStore, int64) {
	t.Helper()
	ctx := context.Background()
	url := testpg.NewDatabase(t)
	if err := migrate.Up(ctx, url); err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	q := store.New(pool)
	a, err := q.CreateAccount(ctx, store.CreateAccountParams{GoogleSub: "bounds", Email: "test@example.invalid"})
	if err != nil {
		t.Fatal(err)
	}
	return ctx, pool, credentialStore{Queries: q, db: pool}, a.ID
}

func TestCredentials_ConcurrentTokenLimit(t *testing.T) {
	ctx, pool, s, id := boundsFixture(t)
	start := make(chan struct{})
	results := make(chan error, 24)
	for i := range 24 {
		go func() {
			<-start
			_, err := s.CreatePersonalToken(ctx, store.CreatePersonalTokenParams{TokenHash: fmt.Sprintf("%064x", i), AccountID: id, Name: "test", ExpiresAt: time.Now().Add(time.Hour)})
			results <- err
		}()
	}
	close(start)
	wins, full := 0, 0
	for range 24 {
		err := <-results
		switch {
		case err == nil:
			wins++
		case errors.Is(err, credential.ErrTokenLimit):
			full++
		default:
			t.Fatal(err)
		}
	}
	if wins != 20 || full != 4 {
		t.Fatalf("created=%d full=%d", wins, full)
	}
	if _, err := pool.Exec(ctx, `UPDATE personal_token SET revoked_at=now() WHERE token_hash=(SELECT token_hash FROM personal_token LIMIT 1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreatePersonalToken(ctx, store.CreatePersonalTokenParams{TokenHash: fmt.Sprintf("%064x", 30), AccountID: id, Name: "replacement", ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
}

func TestCredentials_SessionLimitEvictsOldest(t *testing.T) {
	ctx, pool, s, id := boundsFixture(t)
	m := &websession.Manager{Store: s, PublicURL: "https://example.invalid"}
	w := httptest.NewRecorder()
	if err := m.CreateSession(ctx, w, httptest.NewRequest("GET", "/", nil), id); err != nil {
		t.Fatal(err)
	}
	cookie := w.Result().Cookies()[0]
	r := httptest.NewRequest("GET", "/", nil)
	r.AddCookie(cookie)
	probe := m.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) }))
	w = httptest.NewRecorder()
	probe.ServeHTTP(w, r)
	if w.Code != 204 {
		t.Fatal(w.Code)
	}
	if _, err := pool.Exec(ctx, `UPDATE web_session SET created_at=now()-interval '1 day'`); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for range 55 {
		wg.Go(func() {
			if err := m.CreateSession(ctx, httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil), id); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	var live int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM web_session WHERE account_id=$1 AND revoked_at IS NULL AND expires_at>now()`, id).Scan(&live); err != nil || live != 50 {
		t.Fatalf("live=%d err=%v", live, err)
	}
	w = httptest.NewRecorder()
	probe.ServeHTTP(w, r)
	if w.Code != 401 {
		t.Fatalf("oldest warm session accepted: %d", w.Code)
	}
}

func TestCredentials_ListsAndRetention(t *testing.T) {
	ctx, pool, s, id := boundsFixture(t)
	for _, table := range []string{"personal_token", "web_session"} {
		cols, vals := "name", "'test'"
		if table == "web_session" {
			cols, vals = "csrf_secret", "'secret'"
		}
		sql := fmt.Sprintf(`INSERT INTO %s(token_hash,account_id,%s,auth_epoch,created_at,expires_at,revoked_at)
 SELECT lpad(to_hex(n),64,'0'),$1,%s,0,now()-n*interval '1 hour',
 CASE WHEN n<=110 THEN now()+interval '1 day' ELSE now()-interval '8 days' END,
 CASE WHEN n=111 THEN now()-interval '1 day' ELSE NULL END FROM generate_series(1,112)n`, table, cols, vals)
		if _, err := pool.Exec(ctx, sql, id); err != nil {
			t.Fatal(err)
		}
	}
	tokens, err := s.ListTokens(ctx, id)
	if err != nil || len(tokens) != 100 {
		t.Fatalf("tokens=%d err=%v", len(tokens), err)
	}
	sessions, err := s.ListSessions(ctx, id)
	if err != nil || len(sessions) != 100 {
		t.Fatalf("sessions=%d err=%v", len(sessions), err)
	}
	for i := 1; i < 100; i++ {
		if tokens[i].CreatedAt.After(tokens[i-1].CreatedAt) || sessions[i].CreatedAt.After(sessions[i-1].CreatedAt) {
			t.Fatal("lists not newest first")
		}
	}
	if _, err := s.PurgeEndedCredentials(ctx); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"personal_token", "web_session"} {
		var count int
		if err := pool.QueryRow(ctx, fmt.Sprintf(`SELECT count(*) FROM %s`, table)).Scan(&count); err != nil || count != 110 {
			t.Fatalf("%s count=%d err=%v", table, count, err)
		}
	}
}

func TestCredentials_RecentEndedRows(t *testing.T) {
	ctx, pool, s, id := boundsFixture(t)
	for _, table := range []string{"personal_token", "web_session"} {
		col := "name"
		if table == "web_session" {
			col = "csrf_secret"
		}
		sql := fmt.Sprintf(`INSERT INTO %s(token_hash,account_id,%s,auth_epoch,created_at,expires_at,revoked_at)
 SELECT lpad(to_hex(n),64,'0'),$1,'test',0,now()-interval '10 days',
 CASE n WHEN 2 THEN now()-interval '6 days' WHEN 4 THEN now()-interval '8 days' ELSE now()+interval '1 day' END,
 CASE n WHEN 3 THEN now()-interval '6 days' WHEN 4 THEN now()-interval '1 day' WHEN 5 THEN now()-interval '8 days' END
 FROM generate_series(1,5)n`, table, col)
		if _, err := pool.Exec(ctx, sql, id); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := pool.Exec(ctx, `INSERT INTO web_session(token_hash,account_id,csrf_secret,auth_epoch,created_at,expires_at)
 VALUES (lpad('6',64,'0'),$1,'secret',0,now()-interval '98 days',now()+interval '1 day')`, id); err != nil {
		t.Fatal(err)
	}
	tokens, err := s.ListTokens(ctx, id)
	if err != nil || len(tokens) != 3 {
		t.Fatalf("tokens=%d err=%v", len(tokens), err)
	}
	sessions, err := s.ListSessions(ctx, id)
	if err != nil || len(sessions) != 3 {
		t.Fatalf("sessions=%d err=%v", len(sessions), err)
	}
	if _, err := s.PurgeEndedCredentials(ctx); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"personal_token", "web_session"} {
		var count int
		if err := pool.QueryRow(ctx, fmt.Sprintf(`SELECT count(*) FROM %s`, table)).Scan(&count); err != nil || count != 3 {
			t.Fatalf("%s retained=%d err=%v", table, count, err)
		}
	}
}

func TestCredentials_ExpiredTokensDoNotConsumeLimit(t *testing.T) {
	ctx, pool, s, id := boundsFixture(t)
	if _, err := pool.Exec(ctx, `INSERT INTO personal_token(token_hash,account_id,name,expires_at,auth_epoch)
 SELECT lpad(to_hex(n),64,'0'),$1,'expired',now()-interval '1 day',0 FROM generate_series(1,20)n`, id); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreatePersonalToken(ctx, store.CreatePersonalTokenParams{TokenHash: fmt.Sprintf("%064x", 21), AccountID: id, Name: "live", ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
}

func TestCredentials_RejectsBlockedAccount(t *testing.T) {
	for _, deletion := range []bool{false, true} {
		t.Run(fmt.Sprint(deletion), func(t *testing.T) {
			ctx, _, s, id := boundsFixture(t)
			if deletion {
				session, createErr := s.CreateSession(ctx, store.CreateSessionParams{AccountID: id, TokenHash: strings.Repeat("f", 64), CsrfSecret: "synthetic", ExpiresAt: time.Now().Add(time.Hour)})
				if createErr != nil {
					t.Fatal(createErr)
				}
				if _, err := s.RequestDeletion(ctx, store.RequestDeletionParams{AccountID: id, SessionID: session.ID, AuthEpoch: session.AuthEpoch}); err != nil {
					t.Fatal(err)
				}
			} else {
				if _, err := s.BlockAccount(ctx, id); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := s.CreatePersonalToken(ctx, store.CreatePersonalTokenParams{TokenHash: fmt.Sprintf("%064x", 1), AccountID: id, Name: "test", ExpiresAt: time.Now().Add(time.Hour)}); !errors.Is(err, credential.ErrUnauthorized) {
				t.Fatal("blocked account issued a token")
			}
			if _, err := s.CreateSession(ctx, store.CreateSessionParams{TokenHash: fmt.Sprintf("%064x", 2), AccountID: id, CsrfSecret: "secret", ExpiresAt: time.Now().Add(time.Hour)}); !errors.Is(err, credential.ErrUnauthorized) {
				t.Fatal("blocked account issued a session")
			}
		})
	}
}

func TestCredentials_SessionOverflowRollsBack(t *testing.T) {
	ctx, pool, s, id := boundsFixture(t)
	if _, err := pool.Exec(ctx, `INSERT INTO web_session(token_hash,account_id,csrf_secret,auth_epoch,expires_at)
 SELECT lpad(to_hex(n),64,'0'),$1,'secret',0,now()+interval '1 day' FROM generate_series(1,50)n`, id); err != nil {
		t.Fatal(err)
	}
	_, err := s.CreateSession(ctx, store.CreateSessionParams{TokenHash: fmt.Sprintf("%064x", 1), AccountID: id, CsrfSecret: "secret", ExpiresAt: time.Now().Add(time.Hour)})
	if err == nil {
		t.Fatal("duplicate session created")
	}
	var live int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM web_session WHERE revoked_at IS NULL`).Scan(&live); err != nil || live != 50 {
		t.Fatalf("failed login revoked sessions: live=%d err=%v", live, err)
	}
}
