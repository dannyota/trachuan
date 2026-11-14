//go:build integration

package app

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/password"
	"danny.vn/trachuan/apps/server/internal/store"
)

func passwordAccount(t *testing.T, f *e2eFixture, email, hash string) int64 {
	t.Helper()
	tx, err := f.pool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(t.Context()) }()
	var id int64
	if err = tx.QueryRow(t.Context(), `INSERT INTO account(email) VALUES ($1) RETURNING id`, email).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(t.Context(), `INSERT INTO password_credential(account_id,hash) VALUES ($1,$2)`, id, hash); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	return id
}
func TestPassword_SignInAndReauth(t *testing.T) {
	f := newE2E(t)
	hash, err := password.NewHasher().Hash(t.Context(), "synthetic passphrase")
	if err != nil {
		t.Fatal(err)
	}
	id := passwordAccount(t, f, "person@example.test", hash)
	client := f.client()
	response := f.postWithHeaders(client, "/api/v1/auth/password", `{"email":"person@EXAMPLE.TEST","password":"synthetic passphrase","next":"/oauth/consent/synthetic"}`, "application/json", f.server.URL, "")
	var body struct {
		Next string `json:"next"`
	}
	f.json(response, 200, &body)
	if body.Next != "/oauth/consent/synthetic" {
		t.Fatal("consent destination lost")
	}
	me := f.me(client)
	if me.ID != id {
		t.Fatal("wrong account")
	}
	if _, err = f.pool.Exec(t.Context(), `UPDATE web_session SET proof_at=now()-interval '1 day' WHERE account_id=$1`, id); err != nil {
		t.Fatal(err)
	}
	response = f.postWithHeaders(client, "/api/v1/me/reauth", `{"password":"wrong synthetic password"}`, "application/json", f.server.URL, me.CSRF)
	f.status(response, 401)
	var stale bool
	if err = f.pool.QueryRow(t.Context(), `SELECT proof_at<now()-interval '1 hour' FROM web_session WHERE account_id=$1`, id).Scan(&stale); err != nil || !stale {
		t.Fatal("wrong password refreshed proof")
	}
	f.status(f.postWithHeaders(client, "/api/v1/me/reauth", `{"password":"synthetic passphrase"}`, "application/json", f.server.URL, me.CSRF), 204)
	var fresh bool
	if err = f.pool.QueryRow(t.Context(), `SELECT proof_at>now()-interval '1 minute' AND factor_at IS NULL FROM web_session WHERE account_id=$1`, id).Scan(&fresh); err != nil || !fresh {
		t.Fatal("reauth proof missing")
	}
}
func TestPassword_FailuresAreIdentical(t *testing.T) {
	f := newE2E(t)
	hash, err := password.NewHasher().Hash(t.Context(), "synthetic passphrase")
	if err != nil {
		t.Fatal(err)
	}
	passwordAccount(t, f, "known@example.test", hash)
	blocked := passwordAccount(t, f, "blocked@example.test", hash)
	enrolled := passwordAccount(t, f, "enrolled@example.test", hash)
	if _, err = f.pool.Exec(t.Context(), `UPDATE account SET blocked_at=now() WHERE id=$1`, blocked); err != nil {
		t.Fatal(err)
	}
	if _, err = f.pool.Exec(t.Context(), `INSERT INTO totp_credential(account_id,sealed_secret,key_id) VALUES ($1,'synthetic','synthetic')`, enrolled); err != nil {
		t.Fatal(err)
	}
	if _, err = f.pool.Exec(t.Context(), `INSERT INTO account(google_sub,email) VALUES ('synthetic','google@example.test')`); err != nil {
		t.Fatal(err)
	}
	client := f.client()
	expected := ""
	for _, email := range []string{"unknown@example.test", "google@example.test", "known@example.test", "blocked@example.test", "enrolled@example.test"} {
		secret := "synthetic passphrase"
		if email == "known@example.test" || email == "enrolled@example.test" {
			secret = "incorrect synthetic password"
		}
		response := f.postWithHeaders(client, "/api/v1/auth/password", `{"email":"`+email+`","password":"`+secret+`"}`, "application/json", f.server.URL, "")
		data, readErr := io.ReadAll(response.Body)
		_ = response.Body.Close()
		if readErr != nil || response.StatusCode != 401 || len(response.Cookies()) != 0 {
			t.Fatal("failure issued authority")
		}
		if expected == "" {
			expected = string(data)
		} else if expected != string(data) {
			t.Fatal("non-uniform failure")
		}
	}
	var count int
	if err = f.pool.QueryRow(t.Context(), `SELECT count(*) FROM web_session`).Scan(&count); err != nil || count != 0 {
		t.Fatal("failure wrote session")
	}
}
func TestPassword_CounterSharedWithReauth(t *testing.T) {
	f := newE2E(t)
	hash, err := password.NewHasher().Hash(t.Context(), "synthetic passphrase")
	if err != nil {
		t.Fatal(err)
	}
	passwordAccount(t, f, "person@example.test", hash)
	client := f.client()
	f.status(f.postWithHeaders(client, "/api/v1/auth/password", `{"email":"person@example.test","password":"synthetic passphrase"}`, "application/json", f.server.URL, ""), 200)
	me := f.me(client)
	for range 9 {
		f.status(f.postWithHeaders(client, "/api/v1/me/reauth", `{"password":"wrong synthetic password"}`, "application/json", f.server.URL, me.CSRF), 401)
	}
	f.status(f.postWithHeaders(client, "/api/v1/auth/password", `{"email":"person@EXAMPLE.TEST","password":"synthetic passphrase"}`, "application/json", f.server.URL, ""), 401)
}
func TestPassword_VersionAndSessionRecheckedUnderLock(t *testing.T) {
	for _, reauth := range []bool{false, true} {
		for _, change := range []string{"version", "epoch", "revoke"} {
			t.Run(change+map[bool]string{true: " reauth", false: " sign-in"}[reauth], func(t *testing.T) {
				ctx, pool, _, id := boundsFixture(t)
				if _, err := pool.Exec(ctx, `INSERT INTO password_credential(account_id,hash) VALUES ($1,'synthetic')`, id); err != nil {
					t.Fatal(err)
				}
				account, err := store.New(pool).GetAccount(ctx, id)
				if err != nil {
					t.Fatal(err)
				}
				verified, err := store.New(pool).GetPasswordByEmail(ctx, account.Email)
				if err != nil {
					t.Fatal(err)
				}
				session, err := store.New(pool).CreateSession(ctx, store.CreateSessionParams{AccountID: id, TokenHash: strings.Repeat("a", 64), CsrfSecret: "synthetic", ExpiresAt: time.Now().Add(time.Hour)})
				if err != nil {
					t.Fatal(err)
				}
				observed := &issuerPool{Pool: pool, lock: make(chan struct{})}
				tx, err := pool.Begin(ctx)
				if err != nil {
					t.Fatal(err)
				}
				defer func() { _ = tx.Rollback(ctx) }()
				if _, err = store.LockAccount(ctx, tx, id); err != nil {
					t.Fatal(err)
				}
				switch change {
				case "version":
					_, err = tx.Exec(ctx, `UPDATE password_credential SET version=version+1 WHERE account_id=$1`, id)
				case "epoch":
					_, err = store.BumpEpoch(ctx, tx, id)
				case "revoke":
					_, err = tx.Exec(ctx, `UPDATE web_session SET revoked_at=now() WHERE id=$1`, session.ID)
				}
				if err != nil {
					t.Fatal(err)
				}
				result := make(chan error, 1)
				go func() {
					q := store.New(observed)
					var issueErr error
					if reauth {
						issueErr = q.ReauthPassword(ctx, verified, session.ID)
					} else {
						_, issueErr = q.CreatePasswordSession(ctx, store.PasswordSessionParams{Verified: verified, Session: store.CreateSessionParams{TokenHash: strings.Repeat("b", 64), CsrfSecret: "synthetic", ExpiresAt: time.Now().Add(time.Hour)}})
					}
					result <- issueErr
				}()
				select {
				case <-observed.lock:
				case <-time.After(2 * time.Second):
					t.Fatal("missing account lock")
				}
				if err = tx.Commit(ctx); err != nil {
					t.Fatal(err)
				}
				err = <-result
				if change == "revoke" && !reauth {
					if err != nil {
						t.Fatal(err)
					}
				} else if err != store.ErrLoginFailed {
					t.Fatalf("stale authority accepted: %v", err)
				}
			})
		}
	}
}
func TestPassword_GoogleOnlyReauth(t *testing.T) {
	f := newE2E(t)
	client := f.client()
	f.login(client)
	me := f.me(client)
	response := f.postWithHeaders(client, "/api/v1/me/reauth", `{"password":"synthetic passphrase"}`, "application/json", f.server.URL, me.CSRF)
	data, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil || response.StatusCode != 401 || !strings.Contains(string(data), "reauth_failed") {
		t.Fatal("Google-only reauth accepted")
	}
}

func TestPassword_StaleSessionFailsReauth(t *testing.T) {
	f := newE2E(t)
	hash, err := password.NewHasher().Hash(t.Context(), "synthetic passphrase")
	if err != nil {
		t.Fatal(err)
	}
	id := passwordAccount(t, f, "person@example.test", hash)
	q := store.New(f.pool)
	verified, err := q.GetPasswordByEmail(t.Context(), "person@example.test")
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("POST", "/api/v1/auth/password", nil)
	if err = f.sessions.CreatePasswordSession(t.Context(), recorder, request, verified); err != nil {
		t.Fatal(err)
	}
	var session int64
	if err = f.pool.QueryRow(t.Context(), `SELECT id FROM web_session WHERE account_id=$1`, id).Scan(&session); err != nil {
		t.Fatal(err)
	}
	if _, err = f.pool.Exec(t.Context(), `UPDATE web_session SET expires_at=now()-interval '1 second' WHERE id=$1`, session); err != nil {
		t.Fatal(err)
	}
	if err = q.ReauthPassword(t.Context(), verified, session); err != store.ErrLoginFailed {
		t.Fatal("expired session reauthed")
	}
}
