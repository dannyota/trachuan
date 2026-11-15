//go:build integration

package store_test

import (
	"testing"

	"danny.vn/trachuan/apps/server/internal/frameworkaccess"
	"danny.vn/trachuan/apps/server/internal/store"
)

func TestFrameworkAccess_EmailLifecycle(t *testing.T) {
	ctx, conn, q := setup(t)
	service := frameworkaccess.Service{Store: q, Frameworks: []string{"fw-alpha", "fw-beta"}}
	if _, err := q.AllowFrameworkEntitlement(ctx, store.FrameworkAllowance{Email: " \tOwner@EXAMPLE.test\n", Scope: "fw-alpha", By: "operator"}); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, `INSERT INTO email_link(purpose,token_hash,email,expires_at) VALUES ('registration',repeat('a',64),'Owner@example.test',now()+interval '1 hour')`); err != nil {
		t.Fatal(err)
	}
	set, err := service.AccessSet(ctx, 9999)
	if err != nil || len(set.Scopes()) != 0 || frameworkaccess.Decide("fw-alpha", "licensed", set) != frameworkaccess.Reduced {
		t.Fatal("pending registration gained access")
	}
	a, err := q.CreateAccount(ctx, store.CreateAccountParams{GoogleSub: "synthetic-subject", Email: " \tOwner@EXAMPLE.test\n"})
	if err != nil {
		t.Fatal(err)
	}
	set, err = service.AccessSet(ctx, a.ID)
	if err != nil || frameworkaccess.Decide("fw-alpha", "licensed", set) != frameworkaccess.Full || frameworkaccess.Decide("fw-beta", "licensed", set) != frameworkaccess.Reduced {
		t.Fatal("pre-account entitlement did not apply to canonical email")
	}
	other, err := q.CreateAccount(ctx, store.CreateAccountParams{GoogleSub: "other-subject", Email: "owner@example.test"})
	if err != nil {
		t.Fatal(err)
	}
	set, err = service.AccessSet(ctx, other.ID)
	if err != nil || frameworkaccess.Decide("fw-alpha", "licensed", set) != frameworkaccess.Reduced {
		t.Fatal("case-sensitive local part was ignored")
	}
	if _, err = conn.Exec(ctx, `DELETE FROM account WHERE id=$1`, a.ID); err != nil {
		t.Fatal(err)
	}
	set, err = service.AccessSet(ctx, a.ID)
	if err != nil || len(set.Scopes()) != 0 {
		t.Fatal("deleted account retained access")
	}
	live, err := q.ListLiveFrameworkEntitlementsByEmail(ctx, "Owner@example.test")
	if err != nil || len(live) != 1 {
		t.Fatal("deletion removed entitlement")
	}
	replacement, err := q.CreateAccount(ctx, store.CreateAccountParams{GoogleSub: "replacement-subject", Email: "Owner@example.test"})
	if err != nil {
		t.Fatal(err)
	}
	set, err = service.AccessSet(ctx, replacement.ID)
	if err != nil || frameworkaccess.Decide("fw-alpha", "licensed", set) != frameworkaccess.Full {
		t.Fatal("replacement account did not regain entitlement")
	}
	if err = q.RevokeFrameworkEntitlement(ctx, store.FrameworkRevocation{Email: "Owner@example.test", Scope: "fw-alpha", By: "operator"}); err != nil {
		t.Fatal(err)
	}
	set, err = service.AccessSet(ctx, replacement.ID)
	if err != nil || frameworkaccess.Decide("fw-alpha", "licensed", set) != frameworkaccess.Reduced {
		t.Fatal("revocation was cached")
	}
}

func TestFrameworkAccess_UnknownScopesAndAccountState(t *testing.T) {
	ctx, conn, q := setup(t)
	a, err := q.CreateAccount(ctx, store.CreateAccountParams{GoogleSub: "subject", Email: "owner@example.test"})
	if err != nil {
		t.Fatal(err)
	}
	service := frameworkaccess.Service{Store: q, Frameworks: []string{"fw-alpha"}}
	for _, scope := range []string{"fw-unknown", "*"} {
		if _, err = q.AllowFrameworkEntitlement(ctx, store.FrameworkAllowance{Email: a.Email, Scope: scope, By: "operator"}); err != nil {
			t.Fatal(err)
		}
	}
	set, err := service.AccessSet(ctx, a.ID)
	if err != nil || frameworkaccess.Decide("fw-unknown", "free", set) != frameworkaccess.Reduced || frameworkaccess.Decide("fw-alpha", "licensed", set) != frameworkaccess.Full {
		t.Fatal("unknown framework admitted or wildcard lost")
	}
	for _, column := range []string{"blocked_at", "delete_requested_at"} {
		if _, err = conn.Exec(ctx, `UPDATE account SET `+column+`=now() WHERE id=$1`, a.ID); err != nil {
			t.Fatal(err)
		}
		set, err = service.AccessSet(ctx, a.ID)
		if err != nil || frameworkaccess.Decide("fw-alpha", "licensed", set) != frameworkaccess.Reduced {
			t.Fatal("inactive account gained access")
		}
		if _, err = conn.Exec(ctx, `UPDATE account SET `+column+`=NULL WHERE id=$1`, a.ID); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = conn.Exec(ctx, `UPDATE account SET email='invalid' WHERE id=$1`, a.ID); err != nil {
		t.Fatal(err)
	}
	set, err = service.AccessSet(ctx, a.ID)
	if err == nil || frameworkaccess.Decide("fw-alpha", "licensed", set) != frameworkaccess.Reduced {
		t.Fatal("invalid stored email gained access")
	}
}
