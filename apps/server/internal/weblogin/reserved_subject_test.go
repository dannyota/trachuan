package weblogin

import (
	"testing"

	"danny.vn/trachuan/apps/server/internal/weblogin/oidctest"
)

func TestCallback_RefusesReservedSubjectBeforeAccountAccess(t *testing.T) {
	for _, name := range []string{"new", "existing"} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			subject := "loadcheck:" + name
			if name == "existing" {
				f.store.accounts[subject] = GoogleAccount{ID: 7}
			}
			accounts := len(f.store.accounts)
			a := f.begin(t, "/")
			f.register(a, "reserved-code", func(c *oidctest.Claims) { c.Subject = subject })
			assertRejected(t, f, f.callback(a, "reserved-code"), "login_failed")
			if f.store.upserts != 0 || len(f.store.accounts) != accounts || f.sessions.createCalls != 0 || f.sessions.revokeCalls != 0 {
				t.Fatal("reserved subject reached account or session access")
			}
			assertRejected(t, f, f.callback(a, "reserved-code"), "login_expired")
		})
	}
}
