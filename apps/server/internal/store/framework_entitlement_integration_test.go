//go:build integration

package store_test

import (
	"errors"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/store"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestFrameworkEntitlement_LifecycleAndAudit(t *testing.T) {
	ctx, conn, q := setup(t)
	p := store.FrameworkAllowance{Email: " \tOwner@EXAMPLE.test\n", Scope: "fw-alpha", By: "test-operator", Note: "synthetic note"}
	row, err := q.AllowFrameworkEntitlement(ctx, p)
	if err != nil || row.Email != "Owner@example.test" || row.Scope != "fw-alpha" || row.GrantedBy != "test-operator" || row.Note != "synthetic note" || row.RevokedAt != nil {
		t.Fatalf("row=%+v error=%v", row, err)
	}
	if _, err = q.AllowFrameworkEntitlement(ctx, p); !errors.Is(err, store.ErrFrameworkAlreadyAllowed) || err.Error() != "framework entitlement already allowed" {
		t.Fatalf("duplicate error=%v", err)
	}
	if err = q.RevokeFrameworkEntitlement(ctx, store.FrameworkRevocation{Email: p.Email, Scope: p.Scope, By: "test-revoker"}); err != nil {
		t.Fatal(err)
	}
	if err = q.RevokeFrameworkEntitlement(ctx, store.FrameworkRevocation{Email: p.Email, Scope: p.Scope, By: "test-revoker"}); !errors.Is(err, store.ErrFrameworkNotAllowed) || err.Error() != "framework entitlement not live" {
		t.Fatalf("non-live error=%v", err)
	}
	live, err := q.ListLiveFrameworkEntitlementsByEmail(ctx, "Owner@example.test")
	if err != nil || len(live) != 0 {
		t.Fatalf("revoked rows=%d error=%v", len(live), err)
	}
	var at time.Time
	var by, reason string
	if err = conn.QueryRow(ctx, `SELECT revoked_at,revoked_by,revoke_reason FROM framework_entitlement WHERE id=$1`, row.ID).Scan(&at, &by, &reason); err != nil || by != "test-revoker" || reason != "operator" {
		t.Fatalf("revoke by=%q reason=%q error=%v", by, reason, err)
	}
	events, err := conn.Query(ctx, `SELECT action,email,scope,reason,note,actor,occurred_at FROM framework_entitlement_event ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	defer events.Close()
	count := 0
	for events.Next() {
		var action, email, scope, note, actor string
		var eventReason *string
		var occurred time.Time
		if err = events.Scan(&action, &email, &scope, &eventReason, &note, &actor, &occurred); err != nil {
			t.Fatal(err)
		}
		if email != "Owner@example.test" || scope != "fw-alpha" || note != "synthetic note" || occurred.IsZero() {
			t.Fatal("incomplete audit snapshot")
		}
		if count == 0 && (action != "allow" || eventReason != nil || actor != "test-operator") {
			t.Fatal("incorrect allow event")
		}
		if count == 1 && (action != "revoke" || eventReason == nil || *eventReason != "operator" || actor != "test-revoker" || !occurred.Equal(at)) {
			t.Fatal("incorrect revoke event")
		}
		count++
	}
	if err = events.Err(); err != nil || count != 2 {
		t.Fatalf("events=%d error=%v", count, err)
	}
	events.Close()
	if _, err = q.AllowFrameworkEntitlement(ctx, p); err != nil {
		t.Fatal("could not reallow revoked scope", err)
	}
	rows, err := q.ListFrameworkEntitlements(ctx, "")
	if err != nil || len(rows) != 1 {
		t.Fatalf("live list=%d error=%v", len(rows), err)
	}
	rows, err = q.ListFrameworkEntitlements(ctx, "other@example.test")
	if err != nil || len(rows) != 0 {
		t.Fatalf("filtered list=%d error=%v", len(rows), err)
	}
}

func TestFrameworkEntitlement_DatabaseConstraintsAndImmutability(t *testing.T) {
	ctx, conn, q := setup(t)
	row, err := q.AllowFrameworkEntitlement(ctx, store.FrameworkAllowance{Email: "owner@example.test", Scope: "*", By: "operator"})
	if err != nil {
		t.Fatal(err)
	}
	for _, sql := range []string{
		`UPDATE framework_entitlement_event SET note='changed'`,
		`DELETE FROM framework_entitlement_event`,
		`TRUNCATE framework_entitlement_event`,
		`INSERT INTO framework_entitlement(email,scope,granted_by) VALUES ('owner@example.test','*','operator')`,
		`INSERT INTO framework_entitlement(email,scope,granted_by) VALUES ('owner@EXAMPLE.test','fw-alpha','operator')`,
		`INSERT INTO framework_entitlement(email,scope,granted_by) VALUES ('bad email','fw-alpha','operator')`,
		`INSERT INTO framework_entitlement(email,scope,granted_by) VALUES ('other@example.test','fw--alpha','operator')`,
		`UPDATE framework_entitlement SET revoked_at=now(),revoked_by='operator',revoke_reason='other'`,
		`UPDATE framework_entitlement SET revoked_at=now(),revoked_by='operator'`,
	} {
		if _, err = conn.Exec(ctx, sql); err == nil {
			t.Fatalf("database accepted %s", sql)
		}
	}
	var count int
	if err = conn.QueryRow(ctx, `SELECT count(*) FROM framework_entitlement_event WHERE entitlement_id=$1`, row.ID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("events=%d error=%v", count, err)
	}
}

func TestFrameworkEntitlement_EventFailureRollsBackMutation(t *testing.T) {
	ctx, conn, q := setup(t)
	p := store.FrameworkAllowance{Email: "owner@example.test", Scope: "fw-alpha", By: "operator"}
	if _, err := q.AllowFrameworkEntitlement(ctx, p); err != nil {
		t.Fatal(err)
	}
	_, err := conn.Exec(ctx, `CREATE FUNCTION reject_framework_event() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'synthetic event failure'; END; $$;
 CREATE TRIGGER reject_event BEFORE INSERT ON framework_entitlement_event FOR EACH ROW EXECUTE FUNCTION reject_framework_event()`)
	if err != nil {
		t.Fatal(err)
	}
	p.Scope = "fw-beta"
	if _, err = q.AllowFrameworkEntitlement(ctx, p); err == nil {
		t.Fatal("allow succeeded without audit")
	}
	if err = q.RevokeFrameworkEntitlement(ctx, store.FrameworkRevocation{Email: p.Email, Scope: "fw-alpha", By: p.By}); err == nil {
		t.Fatal("revoke succeeded without audit")
	}
	var live, events int
	if err = conn.QueryRow(ctx, `SELECT (SELECT count(*) FROM framework_entitlement WHERE revoked_at IS NULL),(SELECT count(*) FROM framework_entitlement_event)`).Scan(&live, &events); err != nil || live != 1 || events != 1 {
		t.Fatalf("live=%d events=%d error=%v", live, events, err)
	}
}

func TestFrameworkEntitlement_RejectsInvalidInput(t *testing.T) {
	ctx, conn, q := setup(t)
	for _, p := range []store.FrameworkAllowance{
		{Email: "invalid", Scope: "*", By: "operator"},
		{Email: "owner@example.test", Scope: "fw-*", By: "operator"},
		{Email: "owner@example.test", Scope: "*", By: " \t"},
	} {
		if _, err := q.AllowFrameworkEntitlement(ctx, p); err == nil {
			t.Fatal("invalid allowance accepted")
		}
	}
	var count int
	if err := conn.QueryRow(ctx, `SELECT count(*) FROM framework_entitlement`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("rows=%d error=%v", count, err)
	}
	_, err := conn.Exec(ctx, `INSERT INTO framework_entitlement(email,scope,granted_by) VALUES ('owner@example.test','fw-alpha','operator')`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = conn.Exec(ctx, `INSERT INTO framework_entitlement(email,scope,granted_by) VALUES ('owner@example.test','fw-alpha','operator')`)
	var conflict *pgconn.PgError
	if !errors.As(err, &conflict) || conflict.Code != "23505" {
		t.Fatalf("unique constraint error=%v", err)
	}
}
