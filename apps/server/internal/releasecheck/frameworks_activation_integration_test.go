//go:build integration

package releasecheck

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/migrate"
	"danny.vn/trachuan/internal/testpg"
	"github.com/jackc/pgx/v5"
)

func frameworksSQL(t *testing.T, function string, args ...any) string {
	t.Helper()
	root, err := filepath.Abs("../../../..")
	if err != nil {
		t.Fatal(err)
	}
	if args == nil {
		args = []any{}
	}
	raw, err := json.Marshal(args)
	if err != nil {
		t.Fatal(err)
	}
	script := `import json,sys
sys.path.insert(0,sys.argv[1])
from updater import deploy_frameworks_sql as sql
print(getattr(sql,sys.argv[2])(*json.load(sys.stdin)))
`
	cmd := exec.CommandContext(t.Context(), "python3", "-I", "-B", "-c", script, filepath.Join(root, "deploy/host"), function)
	cmd.Stdin = bytes.NewReader(raw)
	output, err := cmd.Output()
	if err != nil {
		t.Fatal("generate production frameworks activation SQL")
	}
	return string(output)
}

func frameworksRow(operation, rel, mode string, expected any, digest string) map[string]any {
	return map[string]any{"operation_id": strings.Repeat(operation, 32), "country": "vn", "rel": rel, "mode": mode,
		"database_name": "frameworks_" + rel, "manifest_hash": strings.Repeat("a", 64), "dump_sha256": strings.Repeat(digest, 64),
		"expected_active_db": expected, "manifest": map[string]any{"corpus_kind": "frameworks", "rel": rel}}
}

func frameworksAuth(t *testing.T) *pgx.Conn {
	t.Helper()
	ctx := t.Context()
	authURL := testpg.NewDatabase(t)
	if err := migrate.Up(ctx, authURL); err != nil {
		t.Fatal(err)
	}
	auth, err := pgx.Connect(ctx, authURL)
	if err != nil {
		t.Fatal("connect fixture auth")
	}
	t.Cleanup(func() { _ = auth.Close(context.Background()) })
	return auth
}

func frameworksPointer(t *testing.T, conn *pgx.Conn) string {
	t.Helper()
	var pointer string
	err := conn.QueryRow(t.Context(), frameworksSQL(t, "current", "vn")).Scan(&pointer)
	if errors.Is(err, pgx.ErrNoRows) {
		return ""
	}
	if err != nil {
		t.Fatal(err)
	}
	return pointer
}

func TestFrameworksActivation_PointerRetirementProtectionAndPruneAsActivator(t *testing.T) {
	ctx := t.Context()
	builder := testpg.NewBuilder(t)
	auth := frameworksAuth(t)
	role := pgx.Identifier{builder.Role}.Sanitize()
	if _, err := auth.Exec(ctx, "GRANT USAGE ON SCHEMA public TO "+role+"; GRANT SELECT,INSERT,UPDATE,DELETE ON corpus_release,release_journal,retired_release TO "+role+"; GRANT USAGE,SELECT ON SEQUENCE release_journal_id_seq TO "+role+"; GRANT SELECT,INSERT,UPDATE ON frameworks_release TO "+role); err != nil {
		t.Fatal(err)
	}
	activatorConfig, err := pgx.ParseConfig(builder.URL)
	if err != nil {
		t.Fatal("parse synthetic activator configuration")
	}
	activatorConfig.Database = auth.Config().Database
	activator, err := pgx.ConnectConfig(ctx, activatorConfig)
	if err != nil {
		t.Fatal("connect synthetic activator")
	}
	t.Cleanup(func() { _ = activator.Close(context.Background()) })

	if _, err = auth.Exec(ctx, `INSERT INTO corpus_release VALUES ('vn','corpus_vn_20990101',now(),'manual');
 INSERT INTO retired_release (country,db_name,retired_at,retain_until,dropped_at,dump_sha256)
 VALUES ('vn','corpus_vn_20990104',now()-interval '8 days',now()-interval '1 day',NULL,repeat('e',64))`); err != nil {
		t.Fatal(err)
	}
	law := map[string]any{"operation_id": strings.Repeat("9", 32), "country": "vn", "rel": "20990102", "mode": "new", "database_name": "corpus_vn_20990102", "manifest_hash": strings.Repeat("a", 64), "dump_sha256": strings.Repeat("f", 64), "expected_active_db": "corpus_vn_20990101", "manifest": map[string]any{"fixture": true}}
	if _, err = activator.Exec(ctx, pythonSQL(t, "activate", law)); err != nil {
		t.Fatal(err)
	}
	law["operation_id"], law["rel"], law["database_name"], law["expected_active_db"] = strings.Repeat("8", 32), "20990103", "corpus_vn_20990103", "corpus_vn_20990102"
	if _, err = activator.Exec(ctx, pythonSQL(t, "journal", []any{law, "started"})); err != nil {
		t.Fatal(err)
	}
	lawState := func() string {
		t.Helper()
		var value string
		if scanErr := auth.QueryRow(ctx, `SELECT jsonb_build_object(
 'pointer',(SELECT jsonb_agg(to_jsonb(c) ORDER BY country) FROM corpus_release c),
 'journal',(SELECT jsonb_agg(to_jsonb(j) ORDER BY id) FROM release_journal j WHERE corpus_kind='law'),
 'retired',(SELECT jsonb_agg(to_jsonb(r) ORDER BY db_name) FROM retired_release r WHERE corpus_kind='law'))::text`).Scan(&value); scanErr != nil {
			t.Fatal(scanErr)
		}
		return value
	}
	before := lawState()
	if strings.Count(before, `"corpus_kind": "law"`) != 4 {
		t.Fatalf("law fixture rows missing: %s", before)
	}

	apply := func(sql string) {
		t.Helper()
		if _, applyErr := activator.Exec(ctx, sql); applyErr != nil {
			t.Fatal(applyErr)
		}
	}
	refuse := func(sql, cause string) {
		t.Helper()
		_, applyErr := activator.Exec(ctx, sql)
		if applyErr == nil || !strings.Contains(applyErr.Error(), cause) {
			t.Fatalf("activation not refused with %q: %v", cause, applyErr)
		}
		if _, rollbackErr := activator.Exec(ctx, "ROLLBACK"); rollbackErr != nil {
			t.Fatal(rollbackErr)
		}
	}
	journal := func(row map[string]any, step string) {
		t.Helper()
		var raw []byte
		if scanErr := activator.QueryRow(ctx, frameworksSQL(t, "journal", row, step)).Scan(&raw); scanErr != nil {
			t.Fatal(scanErr)
		}
		var stored map[string]any
		if scanErr := json.Unmarshal(raw, &stored); scanErr != nil || stored["corpus_kind"] != "frameworks" || stored["step"] != step || stored["message"] != "frameworks "+step {
			t.Fatalf("journal row %s: %s", step, raw)
		}
	}
	protected := func(name string) bool {
		t.Helper()
		var value bool
		if scanErr := activator.QueryRow(ctx, frameworksSQL(t, "protected", name)).Scan(&value); scanErr != nil {
			t.Fatal(scanErr)
		}
		return value
	}
	retired := func(name string) (string, time.Time, bool) {
		t.Helper()
		var digest *string
		var until time.Time
		scanErr := auth.QueryRow(ctx, `SELECT dump_sha256,retain_until FROM retired_release WHERE corpus_kind='frameworks' AND db_name=$1`, name).Scan(&digest, &until)
		if errors.Is(scanErr, pgx.ErrNoRows) {
			return "", time.Time{}, false
		}
		if scanErr != nil || digest == nil {
			t.Fatalf("retired %s: %v", name, scanErr)
		}
		return *digest, until, true
	}
	expired := func() []string {
		t.Helper()
		var raw []byte
		if scanErr := activator.QueryRow(ctx, frameworksSQL(t, "expired")).Scan(&raw); scanErr != nil {
			t.Fatal(scanErr)
		}
		var names []string
		if scanErr := json.Unmarshal(raw, &names); scanErr != nil {
			t.Fatal(scanErr)
		}
		return names
	}

	if frameworksPointer(t, activator) != "" {
		t.Fatal("fresh auth database has a frameworks pointer")
	}
	first := frameworksRow("1", "20990101", "new", nil, "b")
	journal(first, "started")
	if !protected("frameworks_20990101") {
		t.Fatal("open bootstrap journal target not protected")
	}
	apply(frameworksSQL(t, "activate", first))
	var activatedBy string
	if err = auth.QueryRow(ctx, `SELECT activated_by FROM frameworks_release WHERE country='vn' AND active_db='frameworks_20990101'`).Scan(&activatedBy); err != nil || activatedBy != "activator" {
		t.Fatalf("bootstrap pointer: %v %q", err, activatedBy)
	}
	var count int
	if err = auth.QueryRow(ctx, `SELECT count(*) FROM retired_release WHERE corpus_kind='frameworks'`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("bootstrap retired a database: %d %v", count, err)
	}
	refuse(frameworksSQL(t, "activate", first), "active frameworks changed")

	second := frameworksRow("2", "20990102", "new", "frameworks_20990101", "c")
	journal(second, "started")
	apply(frameworksSQL(t, "activate", second))
	if frameworksPointer(t, activator) != "frameworks_20990102" {
		t.Fatal("pointer not swapped from the journaled value")
	}
	digest, until, ok := retired("frameworks_20990101")
	if !ok || digest != strings.Repeat("b", 64) || until.Before(time.Now().Add(6*24*time.Hour)) {
		t.Fatalf("old frameworks database retirement: ok=%t digest=%s until=%s", ok, digest, until)
	}

	stale := frameworksRow("3", "20990103", "new", "frameworks_20990101", "d")
	refuse(frameworksSQL(t, "activate", stale), "active frameworks changed")
	if frameworksPointer(t, activator) != "frameworks_20990102" {
		t.Fatal("refused swap changed the pointer")
	}
	if err = auth.QueryRow(ctx, `SELECT count(*) FROM release_journal WHERE operation_id=$1`, stale["operation_id"]).Scan(&count); err != nil || count != 0 {
		t.Fatalf("refused swap journaled: %d %v", count, err)
	}

	retained := frameworksRow("4", "20990101", "retained", "frameworks_20990102", "b")
	journal(retained, "started")
	apply(frameworksSQL(t, "activate", retained))
	if frameworksPointer(t, activator) != "frameworks_20990101" {
		t.Fatal("retained reactivation did not move the pointer")
	}
	if _, _, ok = retired("frameworks_20990101"); ok {
		t.Fatal("retained reactivation kept the target's retirement row")
	}
	if digest, _, ok = retired("frameworks_20990102"); !ok || digest != strings.Repeat("c", 64) {
		t.Fatal("retained reactivation did not retire the replaced database with its digest")
	}

	open := frameworksRow("5", "20990103", "new", "frameworks_20990101", "d")
	journal(open, "started")
	for name, want := range map[string]bool{"frameworks_20990101": true, "frameworks_20990102": true, "frameworks_20990103": true, "frameworks_20990104": false} {
		if protected(name) != want {
			t.Fatalf("protection of %s: want %t", name, want)
		}
	}
	journal(open, "aborted")
	if protected("frameworks_20990103") {
		t.Fatal("aborted journal target still protected")
	}
	var operations []byte
	if err = activator.QueryRow(ctx, frameworksSQL(t, "operations")).Scan(&operations); err != nil {
		t.Fatal(err)
	}
	var latest []map[string]any
	if err = json.Unmarshal(operations, &latest); err != nil {
		t.Fatal(err)
	}
	steps := map[string]string{}
	for _, row := range latest {
		if row["corpus_kind"] != "frameworks" {
			t.Fatalf("operations returned a %v row", row["corpus_kind"])
		}
		steps[row["operation_id"].(string)[:1]] = row["step"].(string)
	}
	if want := map[string]string{"1": "activated", "2": "activated", "4": "activated", "5": "aborted"}; len(steps) != len(want) || steps["1"] != want["1"] || steps["2"] != want["2"] || steps["4"] != want["4"] || steps["5"] != want["5"] {
		t.Fatalf("frameworks operations: %v", steps)
	}
	var retirement []byte
	if err = activator.QueryRow(ctx, frameworksSQL(t, "retirement", "frameworks_20990102")).Scan(&retirement); err != nil {
		t.Fatal(err)
	}
	var retention struct {
		Digest string `json:"dump_sha256"`
		Live   bool   `json:"live"`
	}
	if err = json.Unmarshal(retirement, &retention); err != nil || retention.Digest != strings.Repeat("c", 64) || !retention.Live {
		t.Fatalf("retirement read: %v %s", err, retirement)
	}

	if names := expired(); len(names) != 0 {
		t.Fatalf("expired returned %v while the only expired row is law", names)
	}
	if _, err = auth.Exec(ctx, `UPDATE retired_release SET retain_until=now()-interval '1 second' WHERE db_name='frameworks_20990102'`); err != nil {
		t.Fatal(err)
	}
	if names := expired(); len(names) != 1 || names[0] != "frameworks_20990102" || protected("frameworks_20990102") {
		t.Fatalf("expired frameworks database: %v", names)
	}
	prune := map[string]any{"operation_id": strings.Repeat("6", 32), "country": "vn", "rel": "20990102", "mode": "prune", "database_name": "frameworks_20990102", "manifest_hash": nil, "dump_sha256": nil, "expected_active_db": nil, "manifest": nil}
	journal(prune, "pruning")
	apply(frameworksSQL(t, "forget", "frameworks_20990102"))
	journal(prune, "pruned")
	if _, _, ok = retired("frameworks_20990102"); ok || len(expired()) != 0 {
		t.Fatal("forget kept the frameworks retirement row")
	}

	if after := lawState(); after != before {
		t.Fatalf("frameworks statements changed law state:\nbefore %s\nafter  %s", before, after)
	}
}

func TestFrameworksActivation_RefusesUnchangedPointer(t *testing.T) {
	for _, tc := range []struct {
		name, event string
		present     bool
	}{
		{"suppressed_bootstrap_insert", "INSERT", false},
		{"suppressed_swap_update", "UPDATE", true},
		{"bootstrap_with_pointer", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := t.Context()
			conn := frameworksAuth(t)
			var expected any
			if tc.present {
				if _, err := conn.Exec(ctx, `INSERT INTO frameworks_release VALUES ('vn','frameworks_20990101',now(),'fixture')`); err != nil {
					t.Fatal(err)
				}
				if tc.event != "" {
					expected = "frameworks_20990101"
				}
			}
			if tc.event != "" {
				if _, err := conn.Exec(ctx, `CREATE FUNCTION suppress_frameworks_pointer() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RETURN NULL; END; $$;
 CREATE TRIGGER suppress_frameworks_pointer BEFORE `+tc.event+` ON frameworks_release FOR EACH ROW EXECUTE FUNCTION suppress_frameworks_pointer();`); err != nil {
					t.Fatal(err)
				}
			}
			row := frameworksRow("7", "20990102", "new", expected, "b")
			if _, err := conn.Exec(ctx, frameworksSQL(t, "activate", row)); err == nil {
				t.Fatal("activation accepted a missing or unchanged pointer")
			}
			if _, err := conn.Exec(ctx, "ROLLBACK"); err != nil {
				t.Fatal(err)
			}
			var pointer *string
			if err := conn.QueryRow(ctx, `SELECT (SELECT active_db FROM frameworks_release WHERE country='vn')`).Scan(&pointer); err != nil {
				t.Fatal(err)
			}
			if tc.present != (pointer != nil && *pointer == "frameworks_20990101") {
				t.Fatal("refused activation changed the frameworks pointer")
			}
			var journaled, retiredRows int
			if err := conn.QueryRow(ctx, `SELECT (SELECT count(*) FROM release_journal),(SELECT count(*) FROM retired_release)`).Scan(&journaled, &retiredRows); err != nil || journaled != 0 || retiredRows != 0 {
				t.Fatalf("refused activation wrote journal=%d retired=%d: %v", journaled, retiredRows, err)
			}
		})
	}
}
