//go:build integration

package releasecheck

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

const takeActivationLock = `SELECT pg_try_advisory_lock(hashtextextended('activation',0))`
const releaseActivationLock = `SELECT pg_advisory_unlock(hashtextextended('activation',0))`

func (f *phaseFixture) activatorConfig() *pgx.ConnConfig {
	config := f.planted.admin.Copy()
	config.User, config.Password = f.planted.provisioned, f.password
	return config
}

func takeLock(t *testing.T, conn *pgx.Conn) {
	t.Helper()
	var held bool
	if err := conn.QueryRow(t.Context(), takeActivationLock).Scan(&held); err != nil || !held {
		t.Fatalf("take the activation lock: %v %t", err, held)
	}
}

func releaseLock(t *testing.T, conn *pgx.Conn) {
	t.Helper()
	if _, err := conn.Exec(t.Context(), releaseActivationLock); err != nil {
		t.Fatal(err)
	}
}

func (f *phaseFixture) abortedRows(t *testing.T) int {
	t.Helper()
	var count int
	if err := f.planted.auth.QueryRow(t.Context(), `SELECT count(*) FROM public.release_journal WHERE step='aborted'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func TestActivationRecord_AdvisoryLockExcludesAConcurrentRunAcrossCodeShapes(t *testing.T) {
	withLawReleases(t, 2, nil, func(t *testing.T, rels []string) {
		fixture := newPhaseFixture(t, "law", rels, "--no-auth-tables")
		holder, err := pgx.ConnectConfig(t.Context(), fixture.activatorConfig())
		if err != nil {
			t.Fatal("connect lock holder")
		}
		defer func() { _ = holder.Close(context.Background()) }()
		takeLock(t, holder)
		manifest := fixture.manifest(1, rels[0], 2, "b")
		got := fixture.run(t, manifest, nil)
		wantOutcome(t, got, "waiting")
		if !strings.Contains(got.Result.Message, "advisory lock busy") || len(got.Result.Integrity) != 0 || fixture.pointer(t) != fixture.name(0) {
			t.Fatalf("lock held by an activator session: %+v", got)
		}
		releaseLock(t, holder)
		wantOutcome(t, fixture.run(t, manifest, nil), "ok")
	})
}

func TestActivationRecord_LockHeldByAnotherRoleWaitsAndAlerts(t *testing.T) {
	withLawReleases(t, 2, nil, func(t *testing.T, rels []string) {
		fixture := newPhaseFixture(t, "law", rels, "--no-auth-tables")
		owner := fixture.planted.ownerConn
		takeLock(t, owner)
		manifest := fixture.manifest(1, rels[0], 2, "b")
		got := fixture.run(t, manifest, nil)
		wantOutcome(t, got, "waiting")
		if !slices.Contains(got.Result.Integrity, "lock_unavailable") || fixture.pointer(t) != fixture.name(0) {
			t.Fatalf("lock held by the auth owner role: %+v", got)
		}
		releaseLock(t, owner)
		got = fixture.run(t, manifest, nil)
		wantOutcome(t, got, "ok")
		if !slices.Contains(got.Result.Integrity, "lock_unavailable") {
			t.Fatalf("the alert must stay for 24 hours after it was last seen: %+v", got.Result.Integrity)
		}
	})
}

func TestActivationRecord_RefusedAuthConnectionWaitsAndAbortsNothing(t *testing.T) {
	for _, kind := range []string{"law", "frameworks"} {
		t.Run(kind, func(t *testing.T) {
			withLawReleases(t, 2, nil, func(t *testing.T, rels []string) {
				fixture := newPhaseFixture(t, kind, rels, "--no-auth-tables")
				ctx := t.Context()
				manifest := fixture.manifest(1, rels[0], 2, "b")
				got := fixture.run(t, manifest, map[string]any{"crash": "validate"})
				if !got.Crashed || got.Record.Operation == nil || got.Record.Operation.Step != "restored" {
					t.Fatalf("crash left: %+v", got)
				}
				database := pgx.Identifier{fixture.planted.admin.Database}.Sanitize()
				activator := pgx.Identifier{fixture.planted.provisioned}.Sanitize()
				if _, err := fixture.planted.ownerConn.Exec(ctx, "REVOKE CONNECT ON DATABASE "+database+" FROM "+activator+", PUBLIC"); err != nil {
					t.Fatal(err)
				}
				got = fixture.run(t, manifest, nil)
				wantOutcome(t, got, "waiting")
				if !slices.Contains(got.Result.Integrity, "lock_unavailable") {
					t.Fatalf("refused connection raised no alert: %+v", got.Result)
				}
				if got.Record.Operation == nil || got.Record.Operation.Step != "restored" || got.Statuses[got.Digest] != "pending" {
					t.Fatalf("refused connection changed the operation: %+v", got)
				}
				if fixture.pointer(t) != fixture.name(0) || fixture.abortedRows(t) != 0 || !fixture.exists(t, fixture.name(1)) {
					t.Fatal("refused connection aborted the operation or moved the pointer")
				}
				if _, err := fixture.planted.ownerConn.Exec(ctx, "GRANT CONNECT ON DATABASE "+database+" TO "+activator); err != nil {
					t.Fatal(err)
				}
				got = fixture.run(t, manifest, nil)
				wantOutcome(t, got, "ok")
				if fixture.pointer(t) != fixture.name(1) || fixture.abortedRows(t) != 0 {
					t.Fatalf("recovery after the grant: %+v", got)
				}
			})
		})
	}
}
