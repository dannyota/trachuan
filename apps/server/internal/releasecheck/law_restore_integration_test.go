//go:build integration

package releasecheck

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/server/internal/migrate"
	"danny.vn/trachuan/internal/embedding"
	"danny.vn/trachuan/internal/testpg"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestLawRestore_HonestDumpMatchesSourceAndHostileObjectsAreRefused(t *testing.T) {
	dumpPath, restorePath, psqlPath := frameworksClients(t)
	ctx := t.Context()
	admin, _, err := testpg.OpenAdministrator(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Close(context.Background()) })
	activator, reader := testpg.NewBuilder(t), testpg.NewBuilder(t)
	rel := testpg.Release(t)
	name, runKey := "corpus_vn_"+rel, "vn/"+rel
	target := pgx.Identifier{name}.Sanitize()

	source, err := pgx.Connect(ctx, testpg.NewDatabase(t, "vector", "ltree"))
	if err != nil {
		t.Fatal("connect dump source")
	}
	t.Cleanup(func() { _ = source.Close(context.Background()) })
	seedLawCorpus(ctx, t, source, rel)
	sourceConfig := source.Config()
	dump := pgCommand(t, sourceConfig, dumpPath, nil, "-Fc", sourceConfig.Database)
	selected := lawSQL(t, "verify", "toc", string(pgCommand(t, sourceConfig, restorePath, dump, "--list")), []string{"vector", "ltree"})
	listPath := filepath.Join(t.TempDir(), "restore.list")
	if err = os.WriteFile(listPath, []byte(selected), 0o600); err != nil {
		t.Fatal(err)
	}
	var available []byte
	if err = admin.QueryRow(ctx, `SELECT json_object_agg(name,default_version) FROM pg_available_extensions WHERE name IN ('vector','ltree')`).Scan(&available); err != nil {
		t.Fatal(err)
	}
	var versions map[string]string
	if err = json.Unmarshal(available, &versions); err != nil {
		t.Fatal(err)
	}
	catalogPath, schemaPath := lawSQL(t, "sql", "CATALOG_PATH"), lawSQL(t, "sql", "SCHEMA_PATH")

	activatorConfig, err := pgx.ParseConfig(activator.URL)
	if err != nil {
		t.Fatal("parse synthetic activator configuration")
	}
	activatorTarget := activatorConfig.Copy()
	activatorTarget.Database = name
	adminTarget := admin.Config().Copy()
	adminTarget.Database = name
	mark := func() {
		t.Helper()
		if _, markErr := admin.Exec(ctx, "COMMENT ON DATABASE "+target+` IS '{"run_key":"`+runKey+`"}'`); markErr != nil {
			t.Fatal(markErr)
		}
	}
	createTarget := func() {
		t.Helper()
		pgCommand(t, activatorConfig, lawClient(psqlPath, "createdb"), nil, "--template=template0", name)
		mark()
	}
	createTarget()
	t.Cleanup(func() { testpg.DropCorpusDatabase(t, admin, name, runKey) })

	authURL := testpg.NewDatabase(t)
	if err = migrate.Up(ctx, authURL); err != nil {
		t.Fatal(err)
	}
	auth, err := pgx.Connect(ctx, authURL)
	if err != nil {
		t.Fatal("connect fixture auth")
	}
	t.Cleanup(func() { _ = auth.Close(context.Background()) })
	own, other := strings.Repeat("a", 32), strings.Repeat("b", 32)
	row := map[string]any{"operation_id": own, "country": "vn", "rel": rel, "mode": "new", "database_name": name, "manifest_hash": strings.Repeat("c", 64), "dump_sha256": strings.Repeat("d", 64), "expected_active_db": nil, "manifest": map[string]any{"fixture": true}}
	if _, err = auth.Exec(ctx, pythonSQL(t, "journal", []any{row, "creating"})); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		operation any
		want      bool
	}{{nil, true}, {own, false}, {other, true}} {
		var protected bool
		if err = auth.QueryRow(ctx, lawSQL(t, "sql", "protected", name, tc.operation)).Scan(&protected); err != nil {
			t.Fatal(err)
		}
		if protected != tc.want {
			t.Fatalf("protected for operation %v: got %v", tc.operation, protected)
		}
	}

	frameworksPsql(t, psqlPath, activatorTarget, "", "CREATE TABLE pre_created_marker (id integer);\n")
	pgCommand(t, activatorConfig, lawClient(psqlPath, "dropdb"), nil, "--if-exists", name)
	createTarget()
	if got := frameworksPsql(t, psqlPath, activatorTarget, catalogPath, "SELECT pg_catalog.to_regclass('public.pre_created_marker') IS NULL;\n"); got != "t" {
		t.Fatalf("pre-created target kept its marker table: %q", got)
	}

	prepare := lawSQL(t, "sql", "prepare", name, versions)
	adminRole := pgx.Identifier{admin.Config().User}.Sanitize()
	extra := pgx.Identifier{"law_restore_extra_" + strings.ReplaceAll(rel, "_", "")}.Sanitize()
	for _, create := range []string{"CREATE ROLE " + extra + " NOLOGIN SUPERUSER;\n", "CREATE ROLE " + extra + " NOLOGIN;\nGRANT " + adminRole + " TO " + extra + ";\n"} {
		if got := frameworksPsql(t, psqlPath, adminTarget, catalogPath, "BEGIN;\n"+create+prepare+"ROLLBACK;\n"); got != "superuser" {
			t.Fatalf("another superuser not refused: %q", got)
		}
	}
	if got := frameworksPsql(t, psqlPath, adminTarget, catalogPath, prepare); got != "configured" {
		t.Fatalf("pre-restore step: %q", got)
	}

	lawRestore(t, activatorTarget, restorePath, catalogPath, dump, "--dbname", name, "--no-owner", "--no-privileges", "--exit-on-error", "--single-transaction", "--use-list", listPath)
	check := lawSQL(t, "catalog", "law", versions)
	runCheck := func(want string) {
		t.Helper()
		if got := frameworksPsql(t, psqlPath, activatorTarget, catalogPath, "SET standard_conforming_strings=on;\n"+check); got != want {
			t.Fatalf("catalog check: got %q, want %q", got, want)
		}
	}
	runCheck("ok")

	readerConfig, err := pgx.ParseConfig(reader.URL)
	if err != nil {
		t.Fatal("parse synthetic reader")
	}
	readerConfig.Database = name
	early, err := pgx.ConnectConfig(ctx, readerConfig)
	if err == nil {
		_ = early.Close(ctx)
		t.Fatal("reader connected before the owner grants")
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "42501" {
		t.Fatalf("reader refused for another reason: %v", err)
	}
	frameworksPsql(t, psqlPath, activatorTarget, schemaPath, "SET standard_conforming_strings=on;\n"+lawSQL(t, "sql", "grants", name, reader.Role))

	restored, err := pgx.ConnectConfig(ctx, activatorTarget)
	if err != nil {
		t.Fatal("connect restored database")
	}
	t.Cleanup(func() { _ = restored.Close(context.Background()) })
	sourceTables, restoredTables := lawTableDigests(ctx, t, source), lawTableDigests(ctx, t, restored)
	if len(sourceTables) == 0 || !reflect.DeepEqual(sourceTables, restoredTables) {
		t.Fatalf("restored tables differ from the source: %d source, %d restored", len(sourceTables), len(restoredTables))
	}
	if err = restored.Close(ctx); err != nil {
		t.Fatal(err)
	}
	adminSession, err := pgx.ConnectConfig(ctx, adminTarget)
	if err != nil {
		t.Fatal("connect restored database as administrator")
	}
	t.Cleanup(func() { _ = adminSession.Close(context.Background()) })
	var access bool
	if err = adminSession.QueryRow(ctx, `SELECT has_database_privilege($1::name,current_database()::text,'CONNECT')
		AND NOT has_database_privilege('public',current_database()::text,'CONNECT')
		AND has_schema_privilege($1::name,'public','USAGE') AND NOT has_schema_privilege($1::name,'public','CREATE')
		AND NOT has_schema_privilege('public','public','CREATE')
		AND NOT EXISTS (SELECT FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='public' AND CASE
			WHEN c.relkind IN ('r','p') THEN NOT has_table_privilege($1::name,c.oid,'SELECT') OR has_table_privilege($1::name,c.oid,'INSERT,UPDATE,DELETE,TRUNCATE')
			WHEN c.relkind='S' THEN NOT has_sequence_privilege($1::name,c.oid,'SELECT')
			ELSE false END)`, reader.Role).Scan(&access); err != nil {
		t.Fatal(err)
	}
	if !access {
		t.Fatal("reader access differs from the owner grants")
	}

	cfg, err := pgxpool.ParseConfig(strings.Replace(reader.URL, "/postgres", "/"+name, 1))
	if err != nil {
		t.Fatal("parse restored reader configuration")
	}
	cfg.ConnConfig.RuntimeParams["default_transaction_read_only"] = "on"
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal("open restored reader")
	}
	client := embedding.NewClient("http://embedder", &http.Client{Transport: &smokeTransport{}})
	err = checkDatabase(ctx, pool, name, client)
	pool.Close()
	if err != nil {
		t.Fatalf("release-check on the restored database: %v", err)
	}

	prewarm := "SET standard_conforming_strings=on;\n" + lawSQL(t, "sql", "PREWARM")
	skipped := lawSQL(t, "sql", "PREWARM_SKIPPED")
	blocks, err := strconv.ParseInt(frameworksPsql(t, psqlPath, activatorTarget, catalogPath, prewarm), 10, 64)
	if err != nil || blocks <= 0 {
		t.Fatalf("activator prewarm: blocks=%d err=%v", blocks, err)
	}
	if _, err = adminSession.Exec(ctx, "DROP EXTENSION pg_prewarm"); err != nil {
		t.Fatal(err)
	}
	if got := frameworksPsql(t, psqlPath, activatorTarget, catalogPath, prewarm); got != skipped {
		t.Fatalf("prewarm without the extension: %q", got)
	}
	runCheck("ok")
	if _, err = adminSession.Exec(ctx, "CREATE EXTENSION pg_prewarm WITH SCHEMA public"); err != nil {
		t.Fatal(err)
	}
	runCheck("ok")

	activatorRole := pgx.Identifier{activator.Role}.Sanitize()
	for _, tc := range []struct{ plant, undo, want string }{
		{"CREATE FUNCTION public.pg_prewarm(oid) RETURNS bigint LANGUAGE sql AS 'SELECT 0::bigint'", "DROP FUNCTION public.pg_prewarm(oid)", "function"},
		{"ALTER DATABASE " + target + " SET search_path = public, pg_catalog", "ALTER DATABASE " + target + " RESET search_path", "setting"},
		{"ALTER ROLE " + activatorRole + " IN DATABASE " + target + " SET work_mem = '1MB'", "ALTER ROLE " + activatorRole + " IN DATABASE " + target + " RESET work_mem", "setting"},
		{"CREATE FUNCTION invented_extra() RETURNS integer LANGUAGE sql AS 'SELECT 1'", "DROP FUNCTION invented_extra()", "function"},
		{"CREATE VIEW invented_view AS SELECT 1 AS one", "DROP VIEW invented_view", "view"},
		{"CREATE SCHEMA invented_schema", "DROP SCHEMA invented_schema", "schema"},
		{"CREATE EXTENSION pg_trgm", "DROP EXTENSION pg_trgm", "extension"},
		{"CREATE TYPE invented_type AS ENUM ('invented')", "DROP TYPE invented_type", "type"},
		{"CREATE DOMAIN invented_domain AS integer", "DROP DOMAIN invented_domain", "domain"},
		{"CREATE TRIGGER invented_trigger BEFORE UPDATE ON public.release FOR EACH ROW EXECUTE FUNCTION suppress_redundant_updates_trigger()", "DROP TRIGGER invented_trigger ON public.release", "trigger"},
		{"CREATE TABLE public.pg_proc (oid oid, proname name, prokind \"char\", pronamespace oid); CREATE FUNCTION invented_shadowed() RETURNS integer LANGUAGE sql AS 'SELECT 1'", "DROP FUNCTION invented_shadowed(); DROP TABLE public.pg_proc", "function"},
	} {
		frameworksPsql(t, psqlPath, activatorTarget, "", tc.plant+";\n")
		runCheck(tc.want)
		frameworksPsql(t, psqlPath, activatorTarget, "", tc.undo+";\n")
		runCheck("ok")
	}

	frameworksName := "frameworks_" + rel
	frameworksTarget := pgx.Identifier{frameworksName}.Sanitize()
	if _, err = admin.Exec(ctx, "CREATE DATABASE "+frameworksTarget+" TEMPLATE template0"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { testpg.DropCorpusDatabase(t, admin, frameworksName, runKey) })
	if _, err = admin.Exec(ctx, "COMMENT ON DATABASE "+frameworksTarget+` IS '{"run_key":"`+runKey+`"}'`); err != nil {
		t.Fatal(err)
	}
	count := func(databases string) string {
		return "(SELECT count(*) FROM pg_catalog.pg_db_role_setting s WHERE s.setrole='activator'::pg_catalog.regrole AND s.setdatabase IN (" + databases + "))::text"
	}
	reset := "BEGIN;\n" +
		"DO $$ BEGIN IF NOT EXISTS (SELECT FROM pg_catalog.pg_roles WHERE rolname='activator') THEN CREATE ROLE activator NOLOGIN; END IF; END $$;\n" +
		"ALTER ROLE activator SET work_mem = '1MB';\n" +
		"ALTER ROLE activator IN DATABASE " + target + " SET work_mem = '2MB';\n" +
		"ALTER ROLE activator IN DATABASE " + frameworksTarget + " SET work_mem = '3MB';\n" +
		provisionerReset(t) +
		"SELECT " + count("0::pg_catalog.oid,(SELECT oid FROM pg_catalog.pg_database WHERE datname='"+name+"')") + "||','||" + count("(SELECT oid FROM pg_catalog.pg_database WHERE datname='"+frameworksName+"')") + ";\n" +
		"ROLLBACK;\n"
	if got := frameworksPsql(t, psqlPath, adminTarget, schemaPath, reset); got != "0,1" {
		t.Fatalf("provisioner reset left role settings: %q", got)
	}
}
