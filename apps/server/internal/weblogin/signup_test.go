package weblogin

import (
	"testing"

	"danny.vn/trachuan/apps/server/internal/weblogin/oidctest"
)

func TestCallback_SignupEmails(t *testing.T) {
	for _, tc := range []struct {
		name, email string
		list        []string
		existing    *GoogleAccount
		want        string
	}{
		{name: "listed", email: "first@example.test", list: []string{"first@example.test"}, want: "/destination"},
		{name: "email case", email: "FIRST@Example.Test", list: []string{"first@example.test"}, want: "/destination"},
		{name: "unlisted", email: "first@example.test", list: []string{"owner@example.test"}, want: "/login?error=signup_closed"},
		{name: "existing unlisted", email: "new@example.test", list: []string{"owner@example.test"}, existing: &GoogleAccount{ID: 7}, want: "/destination"},
		{name: "blocked unlisted", email: "first@example.test", list: []string{"owner@example.test"}, existing: &GoogleAccount{ID: 7, Blocked: true}, want: "/login?error=account_blocked"},
		{name: "deletion unlisted", email: "first@example.test", list: []string{"owner@example.test"}, existing: &GoogleAccount{ID: 7, DeleteRequested: true}, want: "/login?error=account_blocked"},
		{name: "unset", email: "first@example.test", want: "/destination"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t, tc.list)
			if tc.existing != nil {
				f.store.accounts["test-sub"] = *tc.existing
			}
			a := f.begin(t, "/destination")
			f.register(a, "signup-code", func(c *oidctest.Claims) { c.Email = tc.email })
			w := f.callback(a, "signup-code")
			if w.Code != 302 || w.Header().Get("Location") != tc.want {
				t.Fatalf("response=%d %q", w.Code, w.Header().Get("Location"))
			}
			if tc.want != "/destination" {
				assertNoSession(t, f, w)
			} else {
				if f.sessions.count() != 1 {
					t.Fatal("session not created")
				}
				cookieNamed(t, w, "__Host-session")
				if f.store.emails["test-sub"] != tc.email {
					t.Fatal("current email not stored")
				}
				if tc.existing != nil && f.sessions.accounts[0] != tc.existing.ID {
					t.Fatal("existing subject changed accounts")
				}
			}
			if c := cookieNamed(t, w, "__Host-login"); c.MaxAge >= 0 || c.Value != "" {
				t.Fatal("login cookie not cleared")
			}
			if tc.want == "/login?error=signup_closed" {
				if len(f.store.accounts) != 0 || len(f.store.emails) != 0 {
					t.Fatal("rejected sign-up wrote an account or login email")
				}
				assertRejected(t, f, f.callback(a, "signup-code"), "login_expired")
			}
		})
	}
}

func TestCallback_SignupDatabaseFailureIsUnavailable(t *testing.T) {
	f := newFixture(t, []string{"owner@example.test"})
	a := f.begin(t, "/")
	f.register(a, "signup-code", nil)
	f.store.upsertErr = privateFailure()
	assertUnavailable(t, f, f.callback(a, "signup-code"))
}
