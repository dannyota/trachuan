//go:build integration

package releasecheck

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	schema "danny.vn/trachuan/internal/corpusdb/frameworks"
	"danny.vn/trachuan/internal/embedding"
	"danny.vn/trachuan/internal/testpg"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestFrameworksRestore_UpdaterCommandsRestoreImageObjectsAndLogSettings(t *testing.T) {
	dumpPath, restorePath, psqlPath := frameworksClients(t)
	ctx := t.Context()
	admin, base, err := testpg.OpenAdministrator(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Close(context.Background()) })
	activator, reader := testpg.NewBuilder(t), testpg.NewBuilder(t)
	source, _, _ := frameworkSmokeFixture(t)
	rel := testpg.Release(t)
	name := "frameworks_" + rel
	if _, err = source.Exec(ctx, `CREATE EXTENSION IF NOT EXISTS ltree`); err != nil {
		t.Fatal(err)
	}
	if _, err = source.Exec(ctx, `UPDATE release SET rel=$1`, rel); err != nil {
		t.Fatal(err)
	}
	sourceConfig := source.Config().ConnConfig
	executable, err := schema.ExecutableSQL(schema.SchemaVersion)
	if err != nil {
		t.Fatal(err)
	}
	extensions := []string{"vector", "ltree"}
	dumpSource := func(t *testing.T) []byte {
		return pgCommand(t, sourceConfig, dumpPath, nil, "-Fc", "-Z", "zstd:1", "--no-owner", "--no-privileges", "--no-comments", sourceConfig.Database)
	}
	dump := dumpSource(t)
	selected, refused := frameworksUpdater(t, "schema", "toc", string(pgCommand(t, sourceConfig, restorePath, dump, "--list")), extensions, executable)
	if refused != "" {
		t.Fatalf("updater refused the build dump: %s", refused)
	}
	for _, word := range []string{" VIEW ", " FUNCTION ", " TRIGGER ", " EXTENSION "} {
		if strings.Contains(selected, word) {
			t.Fatalf("restore list keeps %s entries", strings.TrimSpace(word))
		}
	}

	for _, tc := range []struct{ kind, create, drop, cause string }{
		{"function", `CREATE FUNCTION invented_extra() RETURNS integer LANGUAGE sql AS 'SELECT 1'`, `DROP FUNCTION invented_extra()`, "frameworks dump function outside schema version"},
		{"view", `CREATE VIEW invented_extra AS SELECT 1 AS one`, `DROP VIEW invented_extra`, "frameworks dump view outside schema version"},
		{"trigger", `CREATE TRIGGER invented_extra BEFORE DELETE ON framework FOR EACH ROW EXECUTE FUNCTION reject_lifecycle_reset()`, `DROP TRIGGER invented_extra ON framework`, "frameworks dump trigger outside schema version"},
		{"schema", `CREATE SCHEMA invented_extra`, `DROP SCHEMA invented_extra`, "frameworks dump schema other than public"},
	} {
		t.Run("refuses_extra_"+tc.kind, func(t *testing.T) {
			if _, execErr := source.Exec(ctx, tc.create); execErr != nil {
				t.Fatal(execErr)
			}
			tampered := dumpSource(t)
			if _, execErr := source.Exec(ctx, tc.drop); execErr != nil {
				t.Fatal(execErr)
			}
			_, cause := frameworksUpdater(t, "schema", "toc", string(pgCommand(t, sourceConfig, restorePath, tampered, "--list")), extensions, executable)
			if cause != tc.cause {
				t.Fatalf("tampered dump not refused before restore: %q", cause)
			}
		})
	}

	var available []byte
	if err = admin.QueryRow(ctx, `SELECT json_object_agg(name,default_version) FROM pg_available_extensions WHERE name IN ('vector','ltree')`).Scan(&available); err != nil {
		t.Fatal(err)
	}
	var versions map[string]string
	if err = json.Unmarshal(available, &versions); err != nil {
		t.Fatal(err)
	}
	activatorConfig, err := pgx.ParseConfig(activator.URL)
	if err != nil {
		t.Fatal("parse synthetic activator configuration")
	}
	creator, err := pgx.ConnectConfig(ctx, activatorConfig)
	if err != nil {
		t.Fatal("connect synthetic activator")
	}
	_, err = creator.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()+" TEMPLATE template0")
	_ = creator.Close(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = admin.Exec(ctx, "COMMENT ON DATABASE "+pgx.Identifier{name}.Sanitize()+` IS '{"run_key":"vn/`+rel+`"}'`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { testpg.DropCorpusDatabase(t, admin, name, "vn/"+rel) })

	options, _ := frameworksUpdater(t, "sql", "ADMIN_OPTIONS")
	catalogPath, _ := frameworksUpdater(t, "sql", "CATALOG_PATH")
	schemaPath, _ := frameworksUpdater(t, "sql", "SCHEMA_PATH")
	configure, refused := frameworksUpdater(t, "sql", "configure", name, versions, activator.Role, reader.Role)
	if refused != "" {
		t.Fatal(refused)
	}
	adminTarget := admin.Config().Copy()
	adminTarget.Database = name
	if out := frameworksPsql(t, psqlPath, adminTarget, options, configure); out != "configured" {
		t.Fatalf("postgres log settings step: %q", out)
	}
	activatorTarget := activatorConfig.Copy()
	activatorTarget.Database = name
	effective, _ := frameworksUpdater(t, "sql", "effective")
	want, _ := frameworksUpdater(t, "sql", "EFFECTIVE")
	checkSettings := func(t *testing.T, logins ...string) {
		t.Helper()
		if got := frameworksPsql(t, psqlPath, activatorTarget, catalogPath, "SET standard_conforming_strings=on;\n"+effective); got != want {
			t.Fatalf("activator effective log settings %q", got)
		}
		for _, login := range logins {
			config, parseErr := pgx.ParseConfig(login)
			if parseErr != nil {
				t.Fatal("parse synthetic login")
			}
			config.Database = name
			conn, connectErr := pgx.ConnectConfig(ctx, config)
			if connectErr != nil {
				t.Fatal("connect synthetic role")
			}
			settingsErr := schema.CheckLogSettings(ctx, conn)
			_, setErr := conn.Exec(ctx, "SET log_min_messages = 'warning'")
			_ = conn.Close(ctx)
			if settingsErr != nil {
				t.Fatalf("effective log settings refused for %s", config.User)
			}
			var pgErr *pgconn.PgError
			if !errors.As(setErr, &pgErr) || pgErr.Code != "42501" {
				t.Fatalf("non-superuser session override accepted: %v", setErr)
			}
		}
	}
	checkSettings(t, activator.URL)
	for _, role := range []string{activator.Role, reader.Role} {
		if _, err = admin.Exec(ctx, "ALTER ROLE "+pgx.Identifier{role}.Sanitize()+" SET log_min_messages = 'warning'"); err != nil {
			t.Fatal(err)
		}
	}
	checkSettings(t, activator.URL)
	owner, err := pgx.ConnectConfig(ctx, activatorTarget)
	if err != nil {
		t.Fatal("connect synthetic activator to target")
	}
	_, err = owner.Exec(ctx, "ALTER DATABASE "+pgx.Identifier{name}.Sanitize()+" SET log_statement = 'all'")
	_ = owner.Close(ctx)
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "42501" {
		t.Fatalf("database owner changed a log setting: %v", err)
	}

	listPath := filepath.Join(t.TempDir(), "restore.list")
	if err = os.WriteFile(listPath, []byte(selected), 0o600); err != nil {
		t.Fatal(err)
	}
	pgCommand(t, activatorTarget, restorePath, dump, "--dbname", name, "--no-owner", "--no-privileges", "--exit-on-error", "--single-transaction", "--use-list", listPath)
	apply, _ := frameworksUpdater(t, "sql", "apply", executable)
	runApply := func() {
		frameworksPsql(t, psqlPath, activatorTarget, schemaPath, "SET standard_conforming_strings=on;\n"+apply)
	}
	runApply()
	check, refused := frameworksUpdater(t, "check", "executable", executable, versions, activator.Role, reader.Role)
	if refused != "" {
		t.Fatal(refused)
	}
	runCheck := func(want string) {
		t.Helper()
		if got := frameworksPsql(t, psqlPath, activatorTarget, catalogPath, "SET standard_conforming_strings=on;\n"+check); got != want {
			t.Fatalf("restored database check: got %q, want %q", got, want)
		}
	}
	runCheck("ok")
	for _, tc := range []struct{ plant, undo, want string }{
		{"CREATE FUNCTION invented_extra() RETURNS integer LANGUAGE sql AS 'SELECT 1'", "DROP FUNCTION invented_extra()", "function"},
		{"CREATE OR REPLACE VIEW public_lexical_stats AS SELECT count(*) AS chunk_count, coalesce(avg(token_count),0) AS avg_length FROM public_chunk WHERE false", "", "view"},
		{"CREATE SCHEMA invented_extra", "DROP SCHEMA invented_extra", "schema"},
		{"CREATE EXTENSION pg_trgm; CREATE FUNCTION invented_hidden() RETURNS integer LANGUAGE sql AS 'SELECT 1'; ALTER EXTENSION pg_trgm ADD FUNCTION invented_hidden()", "DROP EXTENSION pg_trgm", "extension"},
		{"ALTER DATABASE " + pgx.Identifier{name}.Sanitize() + " SET search_path = public, pg_catalog", "ALTER DATABASE " + pgx.Identifier{name}.Sanitize() + " RESET search_path", "setting"},
		{"CREATE TYPE invented_type AS ENUM ('invented')", "DROP TYPE invented_type", "type"},
		{"CREATE DOMAIN invented_domain AS integer", "DROP DOMAIN invented_domain", "domain"},
		{"CREATE TABLE public.pg_proc (oid oid, proname name, prokind \"char\", pronamespace oid); CREATE FUNCTION invented_extra() RETURNS integer LANGUAGE sql AS 'SELECT 1'", "DROP FUNCTION invented_extra()", "function"},
	} {
		frameworksPsql(t, psqlPath, activatorTarget, "", tc.plant+";\n")
		runCheck(tc.want)
		if tc.undo == "" {
			runApply()
		} else {
			frameworksPsql(t, psqlPath, activatorTarget, "", tc.undo+";\n")
		}
		runCheck("ok")
	}
	frameworksPsql(t, psqlPath, activatorTarget, "", "DROP TABLE public.pg_proc;\n")
	runCheck("ok")
	readerConfig, err := pgx.ParseConfig(reader.URL)
	if err != nil {
		t.Fatal("parse synthetic reader")
	}
	readerConfig.Database = name
	early, err := pgx.ConnectConfig(ctx, readerConfig)
	if err == nil {
		_ = early.Close(ctx)
		t.Fatal("corpus reader connected before the owner grants")
	}
	if !errors.As(err, &pgErr) || pgErr.Code != "42501" {
		t.Fatalf("corpus reader refused for another reason: %v", err)
	}
	grants, _ := frameworksUpdater(t, "sql", "grants", name, reader.Role)
	frameworksPsql(t, psqlPath, activatorTarget, schemaPath, "SET standard_conforming_strings=on;\n"+grants)
	checkSettings(t, activator.URL, reader.URL)

	restoredURL := *base
	restoredURL.Path = "/" + name
	cfg, err := pgxpool.ParseConfig(restoredURL.String())
	if err != nil {
		t.Fatal("parse restored URL")
	}
	restoredAdmin, err := pgx.ConnectConfig(ctx, cfg.ConnConfig.Copy())
	if err != nil {
		t.Fatal("connect restored database")
	}
	restoredObjects := frameworksObjects(t, restoredAdmin)
	_ = restoredAdmin.Close(ctx)
	sourceObjects := frameworksObjects(t, source)
	if len(restoredObjects["view"]) != 4 || len(restoredObjects["function"]) != 6 || len(restoredObjects["trigger"]) != 8 {
		t.Fatalf("restored executable objects: views=%d functions=%d triggers=%d", len(restoredObjects["view"]), len(restoredObjects["function"]), len(restoredObjects["trigger"]))
	}
	restoredJSON, _ := json.Marshal(restoredObjects)
	sourceJSON, _ := json.Marshal(sourceObjects)
	if !bytes.Equal(restoredJSON, sourceJSON) {
		t.Fatal("restored executable objects differ from the image's")
	}

	login, err := pgx.ParseConfig(reader.URL)
	if err != nil {
		t.Fatal("parse synthetic reader")
	}
	cfg.ConnConfig.User, cfg.ConnConfig.Password = login.User, login.Password
	cfg.ConnConfig.RuntimeParams["default_transaction_read_only"] = "on"
	cfg.MaxConns = 2
	guard, err := frameworksDatabaseConfig("postgres://corpus_reader@localhost/{database}", name)
	if err != nil {
		t.Fatal(err)
	}
	cfg.AfterConnect = guard.AfterConnect
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal("open restored reader")
	}
	client := embedding.NewClient("http://embedder", &http.Client{Transport: &smokeTransport{}})
	report, err := checkFrameworksDatabase(ctx, pool, name, client)
	pool.Close()
	if err != nil || report.Status != "ok" || report.SearchHits != 1 || report.ReadItems != 1 {
		t.Fatalf("frameworks release-check on restored database: %+v %v", report, err)
	}

	for _, grantee := range []string{pgx.Identifier{reader.Role}.Sanitize(), "PUBLIC"} {
		revoke := "REVOKE SET ON PARAMETER log_min_messages FROM " + grantee
		t.Cleanup(func() { _, _ = admin.Exec(context.Background(), revoke) })
		if _, err = admin.Exec(ctx, "GRANT SET ON PARAMETER log_min_messages TO "+grantee); err != nil {
			t.Fatal(err)
		}
		out := frameworksPsql(t, psqlPath, adminTarget, options, configure)
		if _, err = admin.Exec(ctx, revoke); err != nil {
			t.Fatal(err)
		}
		if out != "granted" {
			t.Fatalf("parameter grant to %s not refused: %q", grantee, out)
		}
	}
	if out := frameworksPsql(t, psqlPath, adminTarget, options, configure); out != "configured" {
		t.Fatalf("log settings step after revoke: %q", out)
	}
}
