package app

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/store"
	"danny.vn/trachuan/apps/server/internal/weblogin"
	"danny.vn/trachuan/apps/server/internal/websession"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"golang.org/x/oauth2"
)

type deadlineDB struct {
	deadline time.Time
	ok       bool
}
type failedRow struct{}

func (failedRow) Scan(...any) error { return errors.New("database failure") }
func (d *deadlineDB) QueryRow(ctx context.Context, _ string, _ ...any) pgx.Row {
	d.deadline, d.ok = ctx.Deadline()
	return failedRow{}
}
func (*deadlineDB) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	panic("unexpected Exec")
}
func (*deadlineDB) Query(context.Context, string, ...any) (pgx.Rows, error) {
	panic("unexpected Query")
}
func TestLoginStore_QueryDeadline(t *testing.T) {
	for _, tt := range []struct {
		name string
		call func(loginStore) error
	}{
		{"create", func(s loginStore) error {
			return s.CreateLoginTransaction(context.Background(), weblogin.LoginTransaction{})
		}},
		{"claim", func(s loginStore) error {
			_, err := s.ClaimLoginTransaction(context.Background(), weblogin.ClaimLoginTransactionParams{})
			return err
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			db := &deadlineDB{}
			before := time.Now()
			err := tt.call(loginStore{queries: store.New(db)})
			if err == nil {
				t.Fatal("database failure ignored")
			}
			if !db.ok || db.deadline.Before(before) || db.deadline.After(time.Now().Add(authQueryDeadline)) {
				t.Fatal("query lacks two-second deadline")
			}
		})
	}
}

func TestLoginStore_DatabaseFailureIsUnavailable(t *testing.T) {
	sessions := &websession.Manager{Store: routeSessionStore{}, PublicURL: "http://localhost"}
	ctx := context.WithValue(context.Background(), oauth2.HTTPClient, &http.Client{Transport: discoveryRoundTrip{}})
	login, err := weblogin.New(ctx, weblogin.Config{ClientID: "id", ClientSecret: "configured", PublicURL: "http://localhost", Issuer: "https://issuer.test", Store: loginStore{queries: store.New(&deadlineDB{})}, Sessions: sessions})
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name, path string
		handler    http.Handler
		cookie     bool
	}{
		{"create", "/api/v1/auth/google/start", login.Start, false},
		{"claim", "/api/v1/auth/google/callback", login.Callback, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", tt.path, nil)
			if tt.cookie {
				r.AddCookie(&http.Cookie{Name: "__Host-login", Value: strings.Repeat("A", 43)})
			}
			w := httptest.NewRecorder()
			tt.handler.ServeHTTP(w, r)
			if w.Code != 503 || w.Body.String() != "{\"error\":{\"code\":\"unavailable\",\"message\":\"Service unavailable.\"}}\n" {
				t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
			}
		})
	}
}

func TestLoginStore_CanceledContextReturnsError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s := loginStore{queries: store.New(&deadlineDB{})}
	for _, tt := range []struct {
		name string
		call func() error
	}{
		{"create", func() error { return s.CreateLoginTransaction(ctx, weblogin.LoginTransaction{}) }},
		{"claim", func() error {
			_, err := s.ClaimLoginTransaction(ctx, weblogin.ClaimLoginTransactionParams{})
			return err
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.call(); !errors.Is(err, context.Canceled) {
				t.Fatalf("error=%v", err)
			}
		})
	}
}
