//go:build integration

package releasecheck

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

const frameworksExecutableObjects = `SELECT kind, definition FROM (
 SELECT 'view'::text AS kind, c.relname::text||' '||pg_get_viewdef(c.oid) AS definition FROM pg_class c
  WHERE c.relkind IN ('v','m') AND c.relnamespace NOT IN ('pg_catalog'::regnamespace,'information_schema'::regnamespace)
 UNION ALL SELECT 'trigger', pg_get_triggerdef(t.oid) FROM pg_trigger t WHERE NOT t.tgisinternal
 UNION ALL SELECT CASE WHEN p.prokind='f' THEN 'function' ELSE 'routine' END,
  CASE WHEN p.prokind='f' THEN pg_get_functiondef(p.oid) ELSE p.prokind::text||' '||p.proname::text END FROM pg_proc p
  WHERE p.pronamespace NOT IN ('pg_catalog'::regnamespace,'information_schema'::regnamespace)
  AND NOT EXISTS (SELECT FROM pg_depend d WHERE d.classid='pg_proc'::regclass AND d.objid=p.oid AND d.deptype='e')
 UNION ALL SELECT 'rule', r.rulename::text FROM pg_rewrite r WHERE r.rulename <> '_RETURN' AND r.oid >= 16384
 UNION ALL SELECT 'policy', polname::text FROM pg_policy
 UNION ALL SELECT 'event trigger', evtname::text FROM pg_event_trigger
 UNION ALL SELECT 'operator', o.oprname::text FROM pg_operator o WHERE o.oid >= 16384
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
from updater import deploy_frameworks_check, deploy_frameworks_schema, deploy_frameworks_sql
module={'check':deploy_frameworks_check,'schema':deploy_frameworks_schema,'sql':deploy_frameworks_sql}[sys.argv[2]]
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
