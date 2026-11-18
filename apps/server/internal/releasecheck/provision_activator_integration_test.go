//go:build integration

package releasecheck

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/server/internal/migrate"
	"danny.vn/trachuan/internal/testpg"
	"github.com/jackc/pgx/v5"
)

const plantedCatalogFunctions = `
CREATE SCHEMA planted;
CREATE TABLE planted.calls (name text NOT NULL, argument text);
CREATE FUNCTION planted.format(text, text) RETURNS text LANGUAGE plpgsql AS $plant$
BEGIN
  INSERT INTO planted.calls VALUES ('format', $2);
  RETURN pg_catalog.format($1, $2);
END
$plant$;
CREATE FUNCTION planted.to_regclass(text) RETURNS regclass LANGUAGE plpgsql AS $plant$
BEGIN
  INSERT INTO planted.calls VALUES ('to_regclass', $1);
  RETURN pg_catalog.to_regclass($1);
END
$plant$;
CREATE FUNCTION planted.unnest(anyarray) RETURNS SETOF anyelement LANGUAGE plpgsql AS $plant$
BEGIN
  INSERT INTO planted.calls VALUES ('unnest', NULL);
  RETURN QUERY SELECT pg_catalog.unnest($1);
END
$plant$;
`

func provisionName(t *testing.T, prefix string) string {
	t.Helper()
	var raw [10]byte
	if _, err := rand.Read(raw[:]); err != nil {
		t.Fatal(err)
	}
	return prefix + hex.EncodeToString(raw[:])
}

func provisionRoleCleanup(t *testing.T, admin *pgx.Conn, role string) {
	t.Helper()
	t.Cleanup(func() {
		ctx := context.Background()
		if _, err := admin.Exec(ctx, `SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE usename=$1 AND pid <> pg_backend_pid()`, role); err != nil {
			t.Errorf("end provision test sessions: %v", err)
		}
		if _, err := admin.Exec(ctx, "DROP ROLE IF EXISTS "+pgx.Identifier{role}.Sanitize()); err != nil {
			t.Errorf("drop provision test role: %v", err)
		}
	})
}

func provisionScript(t *testing.T, psql, adminUser, host, database, owner, role, secret, ownerSecret string) string {
	t.Helper()
	root, err := filepath.Abs("../../../..")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(root, "deploy/host/provision-activator.sh"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, change := range [][2]string{
		{"/run/secrets/activator_db_password", secret},
		{"/run/secrets/auth_db_password", ownerSecret},
		{"/usr/local/bin/postgres-admin.sh psql", psql},
		{"60s psql", "60s " + psql},
		{"--username postgres", "--username " + adminUser},
		{"--username auth_owner", "--username " + owner},
		{"--host 127.0.0.1", "--host " + host},
		{"--dbname auth", "--dbname " + database},
		{"ON DATABASE auth", "ON DATABASE " + database},
	} {
		if !strings.Contains(text, change[0]) {
			t.Fatalf("provision script no longer contains %q", change[0])
		}
		text = strings.ReplaceAll(text, change[0], change[1])
	}
	return regexp.MustCompile(`\bactivator\b`).ReplaceAllString(text, role)
}

func assertProvisioned(t *testing.T, auth *pgx.Conn, config *pgx.ConnConfig, role, password string) {
	t.Helper()
	ctx := t.Context()
	check := func(label, query string, want bool, args ...any) {
		t.Helper()
		var got bool
		if err := auth.QueryRow(ctx, query, args...).Scan(&got); err != nil {
			t.Fatalf("%s: %v", label, err)
		}
		if got != want {
			t.Errorf("%s = %t; want %t", label, got, want)
		}
	}
	table := `SELECT pg_catalog.has_table_privilege($1, $2, $3)`
	for _, name := range []string{"public.corpus_release", "public.release_journal", "public.retired_release"} {
		for _, privilege := range []string{"SELECT", "INSERT", "UPDATE", "DELETE"} {
			check(name+" "+privilege, table, true, role, name, privilege)
		}
	}
	for privilege, want := range map[string]bool{"SELECT": true, "INSERT": true, "UPDATE": true, "DELETE": false} {
		check("frameworks_release "+privilege, table, want, role, "public.frameworks_release", privilege)
	}
	for _, privilege := range []string{"USAGE", "SELECT"} {
		check("sequence "+privilege, `SELECT pg_catalog.has_sequence_privilege($1, 'public.release_journal_id_seq', $2)`, true, role, privilege)
	}
	check("schema usage", `SELECT pg_catalog.has_schema_privilege($1, 'public', 'USAGE')`, true, role)
	check("database connect", `SELECT pg_catalog.has_database_privilege($1, $2, 'CONNECT')`, true, role, config.Database)
	check("role attributes", `SELECT rolcanlogin AND rolcreatedb AND NOT (rolsuper OR rolcreaterole OR rolreplication OR rolbypassrls) FROM pg_catalog.pg_roles WHERE rolname=$1`, true, role)
	check("scram verifier", `SELECT rolpassword LIKE 'SCRAM-SHA-256$%' FROM pg_catalog.pg_authid WHERE rolname = $1`, true, role)
	login := config.Copy()
	login.User, login.Password = role, password
	conn, err := pgx.ConnectConfig(ctx, login)
	if err != nil {
		t.Fatal("connect with the provisioned password")
	}
	_ = conn.Close(context.Background())
}

func TestProvisionActivator_PinnedSearchPathIgnoresPlantedFunctions(t *testing.T) {
	ctx := t.Context()
	psql := journalPsql(t)
	admin, _, err := testpg.OpenAdministrator(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Close(context.Background()) })
	owner, ownerPassword := provisionName(t, "test_auth_owner_"), provisionName(t, "")
	role, password := provisionName(t, "test_activator_"), provisionName(t, "")
	provisionRoleCleanup(t, admin, owner)
	provisionRoleCleanup(t, admin, role)
	if _, err = admin.Exec(ctx, "CREATE ROLE "+owner+" LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE PASSWORD '"+ownerPassword+"'"); err != nil {
		t.Fatal("create owner role")
	}
	authURL := testpg.NewDatabase(t)
	if err = migrate.Up(ctx, authURL); err != nil {
		t.Fatal(err)
	}
	auth, err := pgx.Connect(ctx, authURL)
	if err != nil {
		t.Fatal("connect fixture auth")
	}
	t.Cleanup(func() { _ = auth.Close(context.Background()) })
	config := auth.Config()
	database := config.Database
	if _, err = auth.Exec(ctx, "ALTER DATABASE "+database+" OWNER TO "+owner); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"corpus_release", "release_journal", "retired_release", "frameworks_release"} {
		if _, err = auth.Exec(ctx, "ALTER TABLE public."+table+" OWNER TO "+owner); err != nil {
			t.Fatal(err)
		}
	}
	ownerConfig := config.Copy()
	ownerConfig.User, ownerConfig.Password = owner, ownerPassword
	ownerConn, err := pgx.ConnectConfig(ctx, ownerConfig)
	if err != nil {
		t.Fatal("connect as the owner role")
	}
	defer func() { _ = ownerConn.Close(context.Background()) }()
	if _, err = ownerConn.Exec(ctx, plantedCatalogFunctions+"ALTER DATABASE "+database+" SET search_path = planted, pg_catalog;ALTER DATABASE "+database+" SET password_encryption = 'md5';"); err != nil {
		t.Fatalf("plant as the owner role: %v", err)
	}

	calls := func() int {
		t.Helper()
		var count int
		if scanErr := auth.QueryRow(ctx, `SELECT pg_catalog.count(*) FROM planted.calls`).Scan(&count); scanErr != nil {
			t.Fatal(scanErr)
		}
		return count
	}
	pgCommand(t, config, psql, []byte("SELECT format('%L', 'control'); SELECT to_regclass('public.release_journal'); SELECT unnest(ARRAY['control']);"), "-X", "-q", "-At", "-v", "ON_ERROR_STOP=1", "-d", database)
	control := calls()
	if control != 3 {
		t.Fatalf("planted functions ran %d times in the unpinned control; want 3", control)
	}

	work := t.TempDir()
	secret := filepath.Join(work, "password")
	if err = os.WriteFile(secret, []byte(password), 0o600); err != nil {
		t.Fatal(err)
	}
	ownerSecret := filepath.Join(work, "owner-password")
	if err = os.WriteFile(ownerSecret, []byte(ownerPassword), 0o600); err != nil {
		t.Fatal(err)
	}
	text := provisionScript(t, psql, config.User, config.Host, database, owner, role, secret, ownerSecret)
	path := filepath.Join(work, "provision-activator.sh")
	if err = os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	for index, mode := range []string{"file", "piped"} {
		t.Run(mode, func(t *testing.T) {
			if index > 0 {
				name := pgx.Identifier{role}.Sanitize()
				if _, resetErr := auth.Exec(t.Context(), "REVOKE ALL ON public.corpus_release, public.release_journal, public.retired_release, public.frameworks_release FROM "+name+"; REVOKE ALL ON SEQUENCE public.release_journal_id_seq FROM "+name+"; REVOKE CONNECT ON DATABASE "+database+" FROM "+name+"; ALTER ROLE "+name+" NOCREATEDB PASSWORD '"+provisionName(t, "")+"'"); resetErr != nil {
					t.Fatal(resetErr)
				}
			}
			cmd := exec.CommandContext(t.Context(), "bash", path)
			if mode == "piped" {
				cmd = exec.CommandContext(t.Context(), "bash", "-s")
				cmd.Stdin = strings.NewReader(text)
			}
			cmd.Env = append(os.Environ(), "PGHOST="+config.Host, "PGPORT="+strconv.Itoa(int(config.Port)), "PGPASSWORD="+config.Password, "PGCONNECT_TIMEOUT=5")
			if output, runErr := cmd.CombinedOutput(); runErr != nil {
				t.Fatalf("run provision script: %v: %s", runErr, strings.ReplaceAll(string(output), password, "[password]"))
			}
			if got := calls(); got != control {
				t.Errorf("planted functions ran %d times during provisioning", got-control)
			}
			var leaked int
			if scanErr := auth.QueryRow(t.Context(), `SELECT pg_catalog.count(*) FROM planted.calls WHERE pg_catalog.strpos(argument, $1) > 0`, password).Scan(&leaked); scanErr != nil {
				t.Fatal(scanErr)
			}
			if leaked != 0 {
				t.Errorf("password reached a planted function %d times", leaked)
			}
			assertProvisioned(t, auth, config, role, password)
		})
	}
}
