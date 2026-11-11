//go:build integration

package accountapi

import (
	"context"
	"encoding/json"
	"errors"
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

func TestAccount_RealStore(t *testing.T) {
	ctx := context.Background()
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
	a, err := q.CreateAccount(ctx, store.CreateAccountParams{GoogleSub: "account-integration", Email: "test@example.invalid"})
	if err != nil {
		t.Fatal(err)
	}
	m := &websession.Manager{Store: q, PublicURL: "https://example.invalid"}
	w := httptest.NewRecorder()
	if err := m.CreateSession(ctx, w, httptest.NewRequest("GET", "https://example.invalid/", nil), a.ID); err != nil {
		t.Fatal(err)
	}
	raw := w.Result().Cookies()[0].Value
	row, err := q.GetSession(ctx, hash(raw))
	if err != nil {
		t.Fatal(err)
	}
	v := &credential.Verifier{Store: q, Audience: "mcp"}
	h := m.Middleware(m.CSRF(New(Config{Store: q, Sessions: m, Verifier: v, Limiter: limits.New(time.Now)})))
	w = call(h, raw, row.CsrfSecret, "POST", "/api/v1/me/tokens", `{"name":"integration"}`)
	expect(t, w, 201)
	var body struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if _, err := v.Verify(ctx, body.Token); err != nil {
		t.Fatal(err)
	}
	expect(t, call(h, raw, row.CsrfSecret, "DELETE", "/api/v1/me", ""), 204)
	if _, err := v.Verify(ctx, body.Token); !errors.Is(err, credential.ErrUnauthorized) {
		t.Fatal("credential remained valid")
	}
	expect(t, call(h, raw, row.CsrfSecret, "GET", "/api/v1/me", ""), 401)
	n, err := Purge(ctx, q, time.Now().Add(6*24*time.Hour))
	if err != nil || n != 0 {
		t.Fatalf("early purge count=%d error=%v", n, err)
	}
	n, err = Purge(ctx, q, time.Now().Add(8*24*time.Hour))
	if err != nil || n != 1 {
		t.Fatalf("purge count=%d error=%v", n, err)
	}
	if _, err := q.GetAccount(ctx, a.ID); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("account not deleted")
	}
}
