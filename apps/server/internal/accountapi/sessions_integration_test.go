//go:build integration

package accountapi

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/credential"
	"danny.vn/trachuan/apps/server/internal/limits"
	"danny.vn/trachuan/apps/server/internal/migrate"
	"danny.vn/trachuan/apps/server/internal/store"
	"danny.vn/trachuan/apps/server/internal/websession"
	"danny.vn/trachuan/internal/testpg"
	"github.com/jackc/pgx/v5"
)

func TestSessions_ListOnlyLive(t *testing.T) {
	ctx := t.Context()
	url := testpg.NewDatabase(t)
	if err := migrate.Up(ctx, url); err != nil {
		t.Fatal(err)
	}
	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close(ctx) })
	q := store.New(conn)
	a, err := q.CreateAccount(ctx, store.CreateAccountParams{GoogleSub: "sessions-fixture", Email: "sessions@example.invalid"})
	if err != nil {
		t.Fatal(err)
	}
	m := &websession.Manager{Store: q, PublicURL: "https://example.invalid"}
	w := httptest.NewRecorder()
	if err := m.CreateSession(ctx, w, httptest.NewRequest("GET", "https://example.invalid/", nil), a.ID); err != nil {
		t.Fatal(err)
	}
	raw := w.Result().Cookies()[0].Value
	current, err := q.GetSession(ctx, hash(raw))
	if err != nil {
		t.Fatal(err)
	}
	var otherLiveID int64
	if err := conn.QueryRow(ctx, `INSERT INTO web_session (token_hash, account_id, csrf_secret, auth_epoch, expires_at)
VALUES (lpad('1', 64, '0'), $1, 'secret', 0, now() + interval '1 day') RETURNING id`, a.ID).Scan(&otherLiveID); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, `INSERT INTO web_session (token_hash, account_id, csrf_secret, auth_epoch, created_at, expires_at, revoked_at)
VALUES (lpad('2', 64, '0'), $1, 'secret', 0, now(), now() + interval '1 day', now()),
       (lpad('3', 64, '0'), $1, 'secret', 0, now(), now() - interval '1 day', NULL),
       (lpad('4', 64, '0'), $1, 'secret', 0, now() - interval '91 days', now() + interval '1 day', NULL)`, a.ID); err != nil {
		t.Fatal(err)
	}
	v := &credential.Verifier{Store: q, Audience: "mcp"}
	h := m.Middleware(m.CSRF(New(Config{Store: q, Sessions: m, Verifier: v, Limiter: limits.New(time.Now)})))
	w = call(h, raw, current.CsrfSecret, "GET", "/api/v1/me/sessions", "")
	expect(t, w, 200)
	var rows []sessionResponse
	if err := json.Unmarshal(w.Body.Bytes(), &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].ID != otherLiveID || rows[0].Current || rows[1].ID != current.ID || !rows[1].Current {
		t.Fatalf("sessions = %+v, want other live session and current session", rows)
	}
}
