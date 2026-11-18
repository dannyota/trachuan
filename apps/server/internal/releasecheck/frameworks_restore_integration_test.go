//go:build integration

package releasecheck

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	schema "danny.vn/trachuan/internal/corpusdb/frameworks"
	"danny.vn/trachuan/internal/embedding"
	"danny.vn/trachuan/internal/testpg"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const frameworksExecutableObjects = `SELECT kind, definition FROM (
 SELECT 'view' AS kind, c.relname||' '||pg_get_viewdef(c.oid) AS definition FROM pg_class c
  WHERE c.relkind IN ('v','m') AND c.relnamespace NOT IN ('pg_catalog'::regnamespace,'information_schema'::regnamespace)
 UNION ALL SELECT 'trigger', pg_get_triggerdef(t.oid) FROM pg_trigger t WHERE NOT t.tgisinternal
 UNION ALL SELECT CASE WHEN p.prokind='f' THEN 'function' ELSE 'routine' END,
  CASE WHEN p.prokind='f' THEN pg_get_functiondef(p.oid) ELSE p.prokind||' '||p.proname END FROM pg_proc p
  WHERE p.pronamespace NOT IN ('pg_catalog'::regnamespace,'information_schema'::regnamespace)
  AND NOT EXISTS (SELECT FROM pg_depend d WHERE d.classid='pg_proc'::regclass AND d.objid=p.oid AND d.deptype='e')
 UNION ALL SELECT 'rule', r.rulename FROM pg_rewrite r WHERE r.rulename <> '_RETURN' AND r.oid >= 16384
 UNION ALL SELECT 'policy', polname FROM pg_policy
 UNION ALL SELECT 'event trigger', evtname FROM pg_event_trigger
 UNION ALL SELECT 'operator', o.oprname FROM pg_operator o WHERE o.oid >= 16384
  AND NOT EXISTS (SELECT FROM pg_depend d WHERE d.classid='pg_operator'::regclass AND d.objid=o.oid AND d.deptype='e')
 UNION ALL SELECT 'cast', format_type(c.castsource,NULL)||' '||format_type(c.casttarget,NULL) FROM pg_cast c WHERE c.oid >= 16384
  AND NOT EXISTS (SELECT FROM pg_depend d WHERE d.classid='pg_cast'::regclass AND d.objid=c.oid AND d.deptype='e')
) objects ORDER BY kind, definition`

func frameworksClients(t *testing.T) (string, string, string) {
	t.Helper()
	dump := pgClientPath("pg_dump", os.Getenv("CORPUS_PG_DUMP"))
	restore := pgClientPath("pg_restore", os.Getenv("CORPUS_PG_RESTORE"))
	for _, client := range []struct{ name, env, path string }{
		{"pg_dump", "CORPUS_PG_DUMP", dump},
		{"pg_restore", "CORPUS_PG_RESTORE", restore},
	} {
		output, err := exec.CommandContext(t.Context(), client.path, "--version").Output()
		if err != nil {
			t.Fatalf("get %s version: %v", client.name, err)
		}
		major, err := pgClientMajorVersion(client.name, string(output))
		if err != nil {
			t.Fatal(err)
		}
		if major < 18 {
			message := fmt.Sprintf("needs %s 18 or newer (found %d); set %s or put /opt/homebrew/opt/postgresql@18/bin first on PATH", client.name, major, client.env)
			if os.Getenv("CI") == "true" {
				t.Fatal(message)
			}
			t.Skip(message)
		}
	}
	psql := "psql"
	if filepath.IsAbs(restore) {
		psql = filepath.Join(filepath.Dir(restore), "psql")
	}
	if _, err := exec.CommandContext(t.Context(), psql, "--version").Output(); err != nil {
		t.Fatalf("get psql version: %v", err)
	}
	return dump, restore, psql
}

func frameworksUpdater(t *testing.T, module, name string, args ...any) (string, string) {
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
from updater import deploy_frameworks_schema, deploy_frameworks_sql
module={'schema':deploy_frameworks_schema,'sql':deploy_frameworks_sql}[sys.argv[2]]
value=getattr(module,sys.argv[3])
try:
    print(json.dumps({'value':value(*json.load(sys.stdin)) if callable(value) else value}))
except ValueError as error:
    print(json.dumps({'refused':str(error)}))
`
	cmd := exec.CommandContext(t.Context(), "python3", "-I", "-B", "-c", script, filepath.Join(root, "deploy/host"), module, name)
	cmd.Stdin = bytes.NewReader(raw)
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("run updater %s.%s", module, name)
	}
	var result struct {
		Value   *string `json:"value"`
		Refused *string `json:"refused"`
	}
	if err = json.Unmarshal(output, &result); err != nil || (result.Value == nil) == (result.Refused == nil) {
		t.Fatalf("decode updater %s.%s", module, name)
	}
	if result.Refused != nil {
		return "", *result.Refused
	}
	return *result.Value, ""
}

func frameworksPsql(t *testing.T, psql string, config *pgx.ConnConfig, options, input string, args ...string) string {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), psql, append([]string{"-X", "-q", "-A", "-t", "-v", "ON_ERROR_STOP=1", "-h", config.Host,
		"-p", strconv.Itoa(int(config.Port)), "-U", config.User, "-d", config.Database}, args...)...)
	cmd.Env = append(os.Environ(), "PGPASSWORD="+config.Password, "PGOPTIONS="+options, "PGCONNECT_TIMEOUT=5")
	cmd.Stdin = strings.NewReader(input)
	output, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			t.Fatalf("fixture psql failed: %v: %.400s", err, bytes.TrimSpace(exitErr.Stderr))
		}
		t.Fatalf("fixture psql failed: %v", err)
	}
	return strings.TrimSpace(string(output))
}

func frameworksObjects(t *testing.T, conn interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}) map[string][]string {
	t.Helper()
	rows, err := conn.Query(t.Context(), frameworksExecutableObjects)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	found := map[string][]string{}
	for rows.Next() {
		var kind, definition string
		if err = rows.Scan(&kind, &definition); err != nil {
			t.Fatal(err)
		}
		found[kind] = append(found[kind], definition)
	}
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	return found
}

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

	for _, tc := range []struct{ kind, create, drop string }{
		{"function", `CREATE FUNCTION invented_extra() RETURNS integer LANGUAGE sql AS 'SELECT 1'`, `DROP FUNCTION invented_extra()`},
		{"view", `CREATE VIEW invented_extra AS SELECT 1 AS one`, `DROP VIEW invented_extra`},
		{"trigger", `CREATE TRIGGER invented_extra BEFORE DELETE ON framework FOR EACH ROW EXECUTE FUNCTION reject_lifecycle_reset()`, `DROP TRIGGER invented_extra ON framework`},
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
			if cause != "frameworks dump "+tc.kind+" outside schema version" {
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

	options, _ := frameworksUpdater(t, "sql", "LOG_OPTIONS")
	configure, refused := frameworksUpdater(t, "sql", "configure", name, versions, []string{activator.Role, reader.Role})
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
	checkSettings := func(t *testing.T) {
		t.Helper()
		if got := frameworksPsql(t, psqlPath, activatorTarget, "", "SET standard_conforming_strings=on;\n"+effective); got != want {
			t.Fatalf("activator effective log settings %q", got)
		}
		for _, login := range []string{activator.URL, reader.URL} {
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
	checkSettings(t)
	for _, role := range []string{activator.Role, reader.Role} {
		if _, err = admin.Exec(ctx, "ALTER ROLE "+pgx.Identifier{role}.Sanitize()+" SET log_min_messages = 'warning'"); err != nil {
			t.Fatal(err)
		}
	}
	checkSettings(t)
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
	frameworksPsql(t, psqlPath, activatorTarget, "", "SET standard_conforming_strings=on;\n"+apply)
	root, err := filepath.Abs("../../../..")
	if err != nil {
		t.Fatal(err)
	}
	script, err := os.ReadFile(filepath.Join(root, "deploy/host/grant-corpus.sh"))
	if err != nil {
		t.Fatal(err)
	}
	blocks := regexp.MustCompile(`(?s)<<'SQL'\n(.*?)\nSQL\n`).FindAllStringSubmatch(string(script), -1)
	if len(blocks) != 2 {
		t.Fatal("grant-corpus.sh SQL blocks changed")
	}
	grants := strings.NewReplacer("corpus_reader", pgx.Identifier{reader.Role}.Sanitize(), "FOR ROLE postgres", "FOR ROLE "+pgx.Identifier{admin.Config().User}.Sanitize())
	adminPostgres := admin.Config().Copy()
	adminPostgres.Database = "postgres"
	frameworksPsql(t, psqlPath, adminPostgres, "", grants.Replace(blocks[0][1])+"\n", "--set", "database="+name)
	frameworksPsql(t, psqlPath, adminTarget, options, grants.Replace(blocks[1][1])+"\n")
	checkSettings(t)

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
