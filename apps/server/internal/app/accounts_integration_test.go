//go:build integration

package app

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/server/internal/authmail"
	"danny.vn/trachuan/apps/server/internal/seal"
	"danny.vn/trachuan/apps/server/internal/store"
)

func TestAccounts_UniformSignupAndReset(t *testing.T) {
	for _, route := range []string{"signup", "reset"} {
		t.Run(route, func(t *testing.T) {
			emails := []string{"unknown@example.test", "password@example.test", "google@example.test", "closed@example.test", "capped@example.test"}
			f := newE2EWithSignupEmails(t, []string{emails[0], emails[1], emails[2], emails[4]})
			q := store.New(f.pool)
			for _, i := range []int{1, 2, 4} {
				a, err := q.CreateAccount(t.Context(), store.CreateAccountParams{GoogleSub: emails[i], Email: emails[i]})
				if err != nil {
					t.Fatal(err)
				}
				if i != 2 {
					if _, err = f.pool.Exec(t.Context(), `INSERT INTO password_credential(account_id,hash) VALUES ($1,'synthetic')`, a.ID); err != nil {
						t.Fatal(err)
					}
				}
				if i == 4 {
					if _, err = f.pool.Exec(t.Context(), `INSERT INTO mail_outbox(kind,class,account_id,sealed_payload,key_id,expires_at) SELECT 'reset','recovery',$1,'synthetic','synthetic',now()+interval '1 day' FROM generate_series(1,3)`, a.ID); err != nil {
						t.Fatal(err)
					}
				}
			}
			var first []byte
			for _, email := range emails {
				response := f.postWithHeaders(f.client(), "/api/v1/auth/"+route, `{"email":"`+email+`"}`, "application/json", f.server.URL, "")
				body, err := io.ReadAll(response.Body)
				_ = response.Body.Close()
				if err != nil || response.StatusCode != 202 || len(response.Cookies()) != 0 || response.Header.Get("Cache-Control") != "no-store" || response.Header.Get("Content-Type") != "application/json" {
					t.Fatal("nonuniform status or headers", response.StatusCode, err)
				}
				if first == nil {
					first = body
				}
				if !bytes.Equal(body, first) || string(body) != "{}\n" {
					t.Fatal("nonuniform body")
				}
			}
			var links, mail int
			if err := f.pool.QueryRow(t.Context(), `SELECT count(*) FROM email_link`).Scan(&links); err != nil {
				t.Fatal(err)
			}
			if err := f.pool.QueryRow(t.Context(), `SELECT count(*) FROM mail_outbox`).Scan(&mail); err != nil {
				t.Fatal(err)
			}
			wantLinks, wantMail := 1, 4
			if route == "signup" {
				wantMail = 6
			}
			if links != wantLinks || mail != wantMail {
				t.Fatalf("links=%d mail=%d", links, mail)
			}
		})
	}
}
func deliveredToken(t *testing.T, f *e2eFixture, kind string) string {
	t.Helper()
	q := store.New(f.pool)
	ring, err := seal.Parse([]byte(base64.RawURLEncoding.EncodeToString(make([]byte, 32))))
	if err != nil {
		t.Fatal(err)
	}
	for range 10 {
		row, claimErr := q.ClaimDueMail(t.Context(), f.sessions.Clock())
		if claimErr != nil {
			t.Fatal(claimErr)
		}
		plain, openErr := q.OpenClaimedMail(t.Context(), ring, row)
		if openErr != nil {
			t.Fatal(openErr)
		}
		var payload authmail.Payload
		if err = json.Unmarshal(plain, &payload); err != nil {
			t.Fatal(err)
		}
		if err = q.FinishMail(t.Context(), store.FinishMailParams{ID: row.ID, Attempts: row.Attempts, Now: f.sessions.Clock(), Sent: true}); err != nil {
			t.Fatal(err)
		}
		if row.Kind == kind {
			return payload.Token
		}
	}
	t.Fatal("link mail missing")
	return ""
}
func TestAccounts_RegistrationResetAndChange(t *testing.T) {
	f := newE2E(t)
	client := f.client()
	post := func(path, body string) *http.Response {
		return f.postWithHeaders(client, path, body, "application/json", f.server.URL, "")
	}
	f.status(post("/api/v1/auth/signup", `{"email":"person@example.test"}`), 202)
	token := deliveredToken(t, f, "registration")
	var link map[string]string
	f.json(post("/api/v1/auth/link", `{"token":"`+token+`"}`), 200, &link)
	if link["purpose"] != "registration" || link["email"] != "person@example.test" {
		t.Fatal("incorrect live link")
	}
	f.status(post("/api/v1/auth/reset/complete", `{"token":"`+token+`","password":"synthetic passphrase alpha"}`), 409)
	f.status(post("/api/v1/auth/signup/complete", `{"token":"`+token+`","password":"short"}`), 400)
	f.status(post("/api/v1/auth/signup/complete", `{"token":"`+token+`","password":"synthetic passphrase alpha","next":"/oauth/consent/ignored"}`), 200)
	me := f.me(client)
	f.status(post("/api/v1/auth/signup/complete", `{"token":"`+token+`","password":"synthetic passphrase alpha"}`), 409)
	response := f.postWithHeaders(client, "/api/v1/me/password", `{"current":"synthetic passphrase alpha","password":"synthetic passphrase beta"}`, "application/json", f.server.URL, me.CSRF)
	f.status(response, 200)
	replacement := f.me(client)
	if replacement.CSRF == me.CSRF {
		t.Fatal("CSRF secret survived password change")
	}
	f.status(f.postWithHeaders(client, "/api/v1/me/reauth", `{"password":"synthetic passphrase beta"}`, "application/json", f.server.URL, me.CSRF), 403)
	f.status(post("/api/v1/auth/reset", `{"email":"person@example.test"}`), 202)
	reset := deliveredToken(t, f, "reset")
	response = post("/api/v1/auth/reset/complete", `{"token":"`+reset+`","password":"synthetic passphrase gamma"}`)
	if len(response.Cookies()) != 0 {
		t.Fatal("reset issued cookie")
	}
	f.status(response, 204)
	f.status(f.request(client, "GET", "/api/v1/me", "", "", "", ""), 401)
	f.status(post("/api/v1/auth/password", `{"email":"person@example.test","password":"synthetic passphrase gamma"}`), 200)
	f.me(client)
}
func TestAccounts_GoogleCanAddPassword(t *testing.T) {
	f := newE2E(t)
	client := f.client()
	f.login(client)
	me := f.me(client)
	f.status(f.postWithHeaders(client, "/api/v1/me/password/link", "", "", f.server.URL, me.CSRF), 202)
	token := deliveredToken(t, f, "add_password")
	f.status(f.postWithHeaders(client, "/api/v1/me/password", `{"token":"`+token+`","password":"synthetic passphrase alpha"}`, "application/json", f.server.URL, me.CSRF), 200)
	if f.me(client).CSRF == me.CSRF {
		t.Fatal("password set did not replace session")
	}
	f.status(f.postWithHeaders(client, "/api/v1/auth/password", `{"email":"`+me.Email+`","password":"synthetic passphrase alpha"}`, "application/json", f.server.URL, ""), 200)
	var count int
	if err := f.pool.QueryRow(t.Context(), `SELECT count(*) FROM mail_outbox WHERE class='notice'`).Scan(&count); err != nil || count != 1 {
		t.Fatal("password set notice count", count, err)
	}
}
func TestAccounts_PreSessionInputAndUniformInvalidAddress(t *testing.T) {
	f := newE2E(t)
	client := f.client()
	for _, route := range []string{"signup", "reset"} {
		for _, body := range []string{`{}`, `{"email":null}`, `{"email":5}`} {
			f.status(f.postWithHeaders(client, "/api/v1/auth/"+route, body, "application/json", f.server.URL, ""), 400)
		}
		f.status(f.postWithHeaders(client, "/api/v1/auth/"+route, `{"email":"invalid"}`, "application/json", f.server.URL, ""), 202)
	}
	f.status(f.postWithHeaders(client, "/api/v1/auth/link", `{"token":"unknown"}`, "application/json", f.server.URL, ""), 409)
	f.status(f.postWithHeaders(client, "/api/v1/auth/link", `{"token":"unknown"}`, "application/json", "https://other.example.test", ""), 403)
	f.status(f.postWithHeaders(client, "/api/v1/auth/link", strings.Repeat("x", 65<<10), "application/json", f.server.URL, ""), 400)
}
