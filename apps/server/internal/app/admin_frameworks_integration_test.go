//go:build integration

package app

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/server/internal/migrate"
	"danny.vn/trachuan/internal/testpg"
	"github.com/jackc/pgx/v5"
)

func TestAdmin_FrameworksLifecycle(t *testing.T) {
	ctx := context.Background()
	url := testpg.NewDatabase(t)
	if err := migrate.Up(ctx, url); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SERVER_AUTH_DATABASE_URL", url)
	var out, stderr bytes.Buffer
	invoke := func(args ...string) error {
		out.Reset()
		return run(ctx, append([]string{"admin", "frameworks"}, args...), &out, &stderr)
	}
	allow := []string{"allow", "--email", " \tOwner@EXAMPLE.test\n", "--scope", "fw-alpha", "--by", "test-operator", "--note", "synthetic private operator note"}
	if err := invoke(allow...); err != nil || out.Len() != 0 {
		t.Fatalf("allow error=%v output=%q", err, out.String())
	}
	if err := invoke(allow...); err == nil || err.Error() != "framework entitlement already allowed" {
		t.Fatalf("duplicate error=%v", err)
	}
	if err := invoke("list"); err != nil {
		t.Fatal(err)
	}
	var row struct{ Email, Scope, Note string }
	if err := json.Unmarshal(bytes.TrimSpace(out.Bytes()), &row); err != nil || row.Email != "Owner@example.test" || row.Scope != "fw-alpha" || strings.Contains(out.String(), "synthetic private operator note") || strings.Contains(out.String(), `"note"`) {
		t.Fatalf("row=%+v error=%v", row, err)
	}
	if err := invoke("list", "--email", "Owner@EXAMPLE.test", "--notes"); err != nil || !strings.Contains(out.String(), "synthetic private operator note") {
		t.Fatal("explicit notes missing", err)
	}
	if err := invoke("list", "--email", "owner@example.test"); err != nil || out.Len() != 0 {
		t.Fatal("different canonical address matched", err)
	}
	revoke := []string{"revoke", "--email", "Owner@example.test", "--scope", "fw-alpha", "--by", "test-revoker"}
	if err := invoke(revoke...); err != nil || out.Len() != 0 {
		t.Fatal("revoke failed", err)
	}
	if err := invoke(revoke...); err == nil || err.Error() != "framework entitlement not live" {
		t.Fatalf("non-live error=%v", err)
	}
	if err := invoke("list"); err != nil || out.Len() != 0 {
		t.Fatal("revoked row listed", err)
	}
	if stderr.Len() != 0 {
		t.Fatal("admin wrote unexpected diagnostics")
	}
}

func TestAdmin_RestoreFence(t *testing.T) {
	ctx := context.Background()
	url := testpg.NewDatabase(t)
	if err := migrate.Up(ctx, url); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SERVER_AUTH_DATABASE_URL", url)
	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close(ctx) }()
	if _, err = conn.Exec(ctx, `INSERT INTO account(google_sub,email) VALUES ('synthetic','owner@example.test')`); err != nil {
		t.Fatal(err)
	}
	var out, stderr bytes.Buffer
	if err = run(ctx, []string{"admin", "frameworks", "allow", "--email", "owner@example.test", "--scope", "*", "--by", "operator"}, &out, &stderr); err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 2; i++ {
		if err = run(ctx, []string{"admin", "restore-fence"}, &out, &stderr); err != nil {
			t.Fatal(err)
		}
		var epoch, live, events int
		if err = conn.QueryRow(ctx, `SELECT auth_epoch,(SELECT count(*) FROM framework_entitlement WHERE revoked_at IS NULL),(SELECT count(*) FROM framework_entitlement_event WHERE reason='restore') FROM account`).Scan(&epoch, &live, &events); err != nil || epoch != i || live != 0 || events != 1 {
			t.Fatalf("epoch=%d live=%d events=%d error=%v", epoch, live, events, err)
		}
	}
	if out.Len() != 0 || stderr.Len() != 0 {
		t.Fatal("restore emitted unexpected output")
	}
}
