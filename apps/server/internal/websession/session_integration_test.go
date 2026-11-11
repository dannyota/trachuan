//go:build integration

package websession

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/migrate"
	"danny.vn/trachuan/apps/server/internal/store"
	"danny.vn/trachuan/internal/testpg"
	"github.com/jackc/pgx/v5"
)

func TestSession_RealStore(t *testing.T) {
	ctx := context.Background()
	dbURL := testpg.NewDatabase(t)
	if err := migrate.Up(ctx, dbURL); err != nil {
		t.Fatal(err)
	}
	conn, err := pgx.Connect(ctx, dbURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close(ctx) })
	q := store.New(conn)
	account, err := q.CreateAccount(ctx, store.CreateAccountParams{GoogleSub: "session-test", Email: "test@example.invalid"})
	if err != nil {
		t.Fatal(err)
	}
	m := &Manager{Store: q, CacheTTL: -1, PublicURL: "https://example.invalid"}
	w := httptest.NewRecorder()
	if err := m.CreateSession(ctx, w, request(""), account.ID); err != nil {
		t.Fatal(err)
	}
	raw := w.Result().Cookies()[0].Value
	row, err := q.GetSession(ctx, hashValue(raw))
	if err != nil || row.AccountID != account.ID || row.TokenHash == raw || len(row.CsrfSecret) != 43 {
		t.Fatalf("stored session account=%d err=%v", row.AccountID, err)
	}
	handler := m.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok := FromContext(r.Context())
		if !ok || p.AccountID != account.ID {
			t.Error("wrong principal")
		}
		w.WriteHeader(204)
	}))
	run := func() *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, request(raw))
		return w
	}
	assertStatus(t, run(), 204)
	if _, err := conn.Exec(ctx, `UPDATE web_session SET created_at=now()-interval '89 days',last_seen_at=now()-interval '2 hours' WHERE id=$1`, row.ID); err != nil {
		t.Fatal(err)
	}
	w = run()
	assertStatus(t, w, 204)
	cookies := w.Result().Cookies()
	if len(cookies) != 1 || cookies[0].MaxAge > 86400 || cookies[0].MaxAge < 86390 {
		t.Fatal("absolute cookie cap")
	}
	updated, err := q.GetSession(ctx, hashValue(raw))
	if err != nil || updated.LastSeenAt.Equal(row.LastSeenAt) || updated.ExpiresAt.After(updated.CreatedAt.Add(90*24*time.Hour)) {
		t.Fatal("touch failed")
	}
	if len(run().Result().Cookies()) != 0 {
		t.Fatal("fresh session renewed")
	}
	count, err := m.RevokeOwnSession(ctx, account.ID, row.ID)
	if err != nil || count != 1 {
		t.Fatalf("revoke count=%d err=%v", count, err)
	}
	assertStatus(t, run(), 401)
}
