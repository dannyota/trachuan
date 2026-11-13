//go:build integration

package app

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/migrate"
	"danny.vn/trachuan/apps/server/internal/store"
	"danny.vn/trachuan/apps/server/internal/weblogin"
	"danny.vn/trachuan/apps/server/internal/weblogin/oidctest"
	"danny.vn/trachuan/apps/server/internal/websession"
	"danny.vn/trachuan/internal/testpg"
	"github.com/jackc/pgx/v5/pgxpool"
)

func signupConfigHandler(t *testing.T, raw *string) (*weblogin.Handler, *pgxpool.Pool, *oidctest.Provider) {
	t.Helper()
	t.Setenv("SERVER_AUTH_DATABASE_URL", "postgres://localhost/auth")
	t.Setenv("SERVER_CORPUS_DATABASE_URL", "postgres://localhost/{database}")
	t.Setenv("SERVER_PUBLIC_URL", "https://example.invalid")
	t.Setenv("SERVER_GOOGLE_ISSUER", "")
	t.Setenv("SERVER_OAUTH_CLIENTS", "")
	t.Setenv("SERVER_SIGNUP_EMAILS", "")
	if raw == nil {
		if err := os.Unsetenv("SERVER_SIGNUP_EMAILS"); err != nil {
			t.Fatal(err)
		}
	} else {
		t.Setenv("SERVER_SIGNUP_EMAILS", *raw)
	}
	cfg, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	dsn := testpg.NewDatabase(t)
	if err = migrate.Up(t.Context(), dsn); err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.New(t.Context(), dsn)
	if err != nil {
		t.Fatal("connect to sign-up test database")
	}
	t.Cleanup(pool.Close)
	q := store.New(pool)
	p := oidctest.New(t)
	h, err := weblogin.New(t.Context(), weblogin.Config{ClientID: oidctest.ClientID, ClientSecret: "fixture", PublicURL: cfg.publicURL, Issuer: p.URL, SignupEmails: cfg.signupEmails, Store: loginStore{q}, Sessions: &websession.Manager{Store: q, PublicURL: cfg.publicURL}})
	if err != nil {
		t.Fatal(err)
	}
	return h, pool, p
}

func signupRequest(t *testing.T, h *weblogin.Handler, p *oidctest.Provider, code, sub, email string) *http.Request {
	t.Helper()
	w := httptest.NewRecorder()
	h.Start.ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/auth/google/start?next=/destination", nil))
	if w.Code != 302 {
		t.Fatalf("start status %d", w.Code)
	}
	u, err := url.Parse(w.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	p.RegisterCode(code, oidctest.Claims{Subject: sub, Email: email, EmailVerified: true, Nonce: q.Get("nonce"), Challenge: q.Get("code_challenge"), RedirectURI: q.Get("redirect_uri")})
	r := httptest.NewRequest("GET", "/api/v1/auth/google/callback?"+url.Values{"code": {code}, "state": {q.Get("state")}}.Encode(), nil)
	for _, cookie := range w.Result().Cookies() {
		r.AddCookie(cookie)
	}
	return r
}

func assertSignupResponse(t *testing.T, w *httptest.ResponseRecorder, want string, session bool) {
	t.Helper()
	if w.Code != 302 || w.Header().Get("Location") != want {
		t.Fatalf("callback=%d %q", w.Code, w.Header().Get("Location"))
	}
	var cleared, issued bool
	for _, c := range w.Result().Cookies() {
		if c.Name == "__Host-login" && c.Value == "" && c.MaxAge < 0 {
			cleared = true
		}
		if c.Name == "__Host-session" {
			issued = true
		}
	}
	if !cleared || issued != session {
		t.Fatal("incorrect login or session cookies")
	}
}

func assertSignupRows(t *testing.T, pool *pgxpool.Pool, accounts, sessions, claimed int) {
	t.Helper()
	var gotAccounts, gotSessions, gotClaimed int
	err := pool.QueryRow(t.Context(), `SELECT (SELECT count(*) FROM account), (SELECT count(*) FROM web_session), (SELECT count(*) FROM login_transaction WHERE claimed_at IS NOT NULL)`).Scan(&gotAccounts, &gotSessions, &gotClaimed)
	if err != nil {
		t.Fatal("count sign-up rows")
	}
	if gotAccounts != accounts || gotSessions != sessions || gotClaimed != claimed {
		t.Fatalf("rows: accounts=%d sessions=%d claimed=%d", gotAccounts, gotSessions, gotClaimed)
	}
}

func TestConfig_SignupCallbackDatabase(t *testing.T) {
	for _, tc := range []struct {
		name, list, email, want                        string
		open, existing, blocked, deleted, otherSubject bool
	}{
		{name: "listed", list: "fixture@example.invalid", email: "fixture@example.invalid", want: "/destination"},
		{name: "list case and whitespace", list: " OTHER@example.invalid, FIXTURE@Example.Invalid ", email: "fixture@example.invalid", want: "/destination"},
		{name: "email case", list: "fixture@example.invalid", email: "FIXTURE@Example.Invalid", want: "/destination"},
		{name: "unlisted", list: "owner@example.invalid", email: "fixture@example.invalid", want: "/login?error=signup_closed"},
		{name: "existing unlisted", list: "owner@example.invalid", email: "current@example.invalid", existing: true, want: "/destination"},
		{name: "blocked unlisted", list: "owner@example.invalid", email: "current@example.invalid", existing: true, blocked: true, want: "/login?error=account_blocked"},
		{name: "deleted unlisted", list: "owner@example.invalid", email: "current@example.invalid", existing: true, deleted: true, want: "/login?error=account_blocked"},
		{name: "same email other subject", list: "owner@example.invalid", email: "fixture@example.invalid", otherSubject: true, want: "/login?error=signup_closed"},
		{name: "unset", open: true, email: "fixture@example.invalid", want: "/destination"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var list *string
			if !tc.open {
				list = &tc.list
			}
			h, pool, p := signupConfigHandler(t, list)
			const sub = "signup-subject"
			var id int64
			old := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
			if tc.existing || tc.otherSubject {
				seedSub, seedEmail := sub, "old@example.invalid"
				if tc.otherSubject {
					seedSub, seedEmail = "other-subject", tc.email
				}
				err := pool.QueryRow(t.Context(), `INSERT INTO account (google_sub, email, last_login_at, blocked_at, delete_requested_at) VALUES ($1,$2,$3,CASE WHEN $4 THEN now() END,CASE WHEN $5 THEN now() END) RETURNING id`, seedSub, seedEmail, old, tc.blocked, tc.deleted).Scan(&id)
				if err != nil {
					t.Fatal("seed sign-up account")
				}
			}
			w := httptest.NewRecorder()
			h.Callback.ServeHTTP(w, signupRequest(t, h, p, "signup-code", sub, tc.email))
			success := tc.want == "/destination"
			assertSignupResponse(t, w, tc.want, success)
			accounts, sessions := 0, 0
			if tc.existing || tc.otherSubject || success {
				accounts = 1
			}
			if success {
				sessions = 1
			}
			assertSignupRows(t, pool, accounts, sessions, 1)
			if success || tc.existing {
				var gotID int64
				var email string
				var lastLogin time.Time
				if err := pool.QueryRow(t.Context(), `SELECT id,email,last_login_at FROM account WHERE google_sub=$1`, sub).Scan(&gotID, &email, &lastLogin); err != nil {
					t.Fatal("read signed-in account")
				}
				if email != tc.email || !lastLogin.After(old) || (tc.existing && gotID != id) {
					t.Fatal("account identity, current email or login time not retained")
				}
			}
		})
	}
}

func TestConfig_SignupConcurrentCallbacks(t *testing.T) {
	for _, listed := range []bool{false, true} {
		t.Run(strconv.FormatBool(listed), func(t *testing.T) {
			list := "owner@example.invalid"
			if listed {
				list = "fixture@example.invalid"
			}
			h, pool, p := signupConfigHandler(t, &list)
			requests := make([]*http.Request, 2)
			for i := range requests {
				requests[i] = signupRequest(t, h, p, "concurrent-"+strconv.Itoa(i), "new-subject", "fixture@example.invalid")
			}
			results := make([]*httptest.ResponseRecorder, 2)
			start := make(chan struct{})
			var wg sync.WaitGroup
			for i := range requests {
				wg.Go(func() { <-start; results[i] = httptest.NewRecorder(); h.Callback.ServeHTTP(results[i], requests[i]) })
			}
			close(start)
			wg.Wait()
			want, accounts, sessions := "/login?error=signup_closed", 0, 0
			if listed {
				want, accounts, sessions = "/destination", 1, 2
			}
			for _, w := range results {
				assertSignupResponse(t, w, want, listed)
			}
			assertSignupRows(t, pool, accounts, sessions, 2)
		})
	}
}
