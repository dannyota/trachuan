//go:build integration

package releasecheck

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"danny.vn/trachuan/internal/testpg"
	"github.com/jackc/pgx/v5"
)

const plantedAttempt = `
CREATE TABLE public.planted_log (id bigserial PRIMARY KEY, who text NOT NULL, outcome text NOT NULL);
CREATE FUNCTION public.planted_attempt() RETURNS bigint LANGUAGE plpgsql AS $plant$
BEGIN
  BEGIN
    EXECUTE 'ALTER ROLE VICTIM_ROLE SUPERUSER';
    INSERT INTO public.planted_log (who, outcome) VALUES (current_user, 'escalated');
  EXCEPTION WHEN insufficient_privilege THEN
    INSERT INTO public.planted_log (who, outcome) VALUES (current_user, 'denied');
  END;
  RETURN 42;
END
$plant$;
`

type plantedEntry struct{ who, outcome string }

type plantedAuth struct {
	auth, ownerConn *pgx.Conn
	admin, owner    *pgx.ConnConfig
	ownerRole       string
	victim          string
	provisioned     string
	psql            string
}

func plantedName(t *testing.T, prefix string) string {
	t.Helper()
	var raw [10]byte
	if _, err := rand.Read(raw[:]); err != nil {
		t.Fatal(err)
	}
	return prefix + hex.EncodeToString(raw[:])
}

func plantedRole(t *testing.T, admin *pgx.Conn, prefix, attributes string) (string, string) {
	t.Helper()
	role, password := plantedName(t, prefix), plantedName(t, "")
	name := pgx.Identifier{role}.Sanitize()
	t.Cleanup(func() {
		ctx := context.Background()
		if _, err := admin.Exec(ctx, `SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE usename=$1 AND pid <> pg_backend_pid()`, role); err != nil {
			t.Errorf("end planted role sessions: %v", err)
		}
		if _, err := admin.Exec(ctx, "DROP ROLE IF EXISTS "+name); err != nil {
			t.Errorf("drop planted role: %v", err)
		}
	})
	if _, err := admin.Exec(t.Context(), "CREATE ROLE "+name+" "+attributes+" PASSWORD '"+password+"'"); err != nil {
		t.Fatalf("create planted role: %v", err)
	}
	return role, password
}

func newPlantedAuth(t *testing.T) *plantedAuth {
	t.Helper()
	ctx := t.Context()
	_, _, psql := frameworksClients(t)
	admin, _, err := testpg.OpenAdministrator(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Close(context.Background()) })
	owner, password := plantedRole(t, admin, "test_auth_owner_", "LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS")
	victim, _ := plantedRole(t, admin, "test_auth_victim_", "NOLOGIN NOSUPERUSER")
	provisioned, _ := plantedRole(t, admin, "test_provisioned_", "NOLOGIN")
	authURL := testpg.NewDatabase(t)
	auth, err := pgx.Connect(ctx, authURL)
	if err != nil {
		t.Fatal("connect planted auth")
	}
	t.Cleanup(func() { _ = auth.Close(context.Background()) })
	adminConfig := auth.Config()
	if _, err = auth.Exec(ctx, "ALTER DATABASE "+pgx.Identifier{adminConfig.Database}.Sanitize()+" OWNER TO "+pgx.Identifier{owner}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	ownerConfig := adminConfig.Copy()
	ownerConfig.User, ownerConfig.Password = owner, password
	ownerConn, err := pgx.ConnectConfig(ctx, ownerConfig)
	if err != nil {
		t.Fatal("connect planted owner")
	}
	t.Cleanup(func() { _ = ownerConn.Close(context.Background()) })
	fixture := &plantedAuth{auth: auth, ownerConn: ownerConn, admin: adminConfig, owner: ownerConfig,
		ownerRole: owner, victim: victim, provisioned: provisioned, psql: psql}
	fixture.plant(t, strings.ReplaceAll(plantedAttempt, "VICTIM_ROLE", pgx.Identifier{victim}.Sanitize()))
	return fixture
}

func (p *plantedAuth) plant(t *testing.T, statements string) {
	t.Helper()
	if _, err := p.ownerConn.Exec(t.Context(), statements); err != nil {
		t.Fatalf("plant as the owner role: %v", err)
	}
}

func (p *plantedAuth) log(t *testing.T) []plantedEntry {
	t.Helper()
	rows, err := p.auth.Query(t.Context(), `SELECT who, outcome FROM public.planted_log ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (plantedEntry, error) {
		var entry plantedEntry
		return entry, row.Scan(&entry.who, &entry.outcome)
	})
	if err != nil {
		t.Fatal(err)
	}
	return entries
}

func (p *plantedAuth) victimIsSuperuser(t *testing.T) bool {
	t.Helper()
	var super bool
	if err := p.auth.QueryRow(t.Context(), `SELECT rolsuper FROM pg_catalog.pg_roles WHERE rolname=$1`, p.victim).Scan(&super); err != nil {
		t.Fatal(err)
	}
	return super
}

type updaterAuthRead struct{ sql, options string }

func authReadFromUpdater(t *testing.T, constant string) updaterAuthRead {
	t.Helper()
	script := `import json,sys
sys.path.insert(0,sys.argv[1])
from updater import deploy_corpus_sql as sql, deploy_host as host
print(json.dumps(host.auth_arguments(getattr(sql,sys.argv[2]))))
`
	output, err := exec.CommandContext(t.Context(), "python3", "-I", "-B", "-c", script, filepath.Join(repositoryRoot(t), "deploy/host"), constant).Output()
	if err != nil {
		t.Fatalf("read updater auth arguments for %s", constant)
	}
	var args []string
	if err = json.Unmarshal(output, &args); err != nil {
		t.Fatal("decode updater auth arguments")
	}
	program := slices.Index(args, "psql")
	if program < 1 || !strings.HasPrefix(args[program-1], "PGOPTIONS=") {
		t.Fatalf("updater auth arguments have no psql session: %q", args)
	}
	if !slices.ContainsFunc(args[:program], func(arg string) bool { return strings.Contains(arg, "/run/secrets/auth_db_password") }) {
		t.Fatal("updater auth session does not read the auth_owner password")
	}
	flags := args[program+1:]
	value := func(flag string) string {
		t.Helper()
		index := slices.Index(flags, flag)
		if index < 0 || index+1 == len(flags) {
			t.Fatalf("updater auth arguments have no %s", flag)
		}
		return flags[index+1]
	}
	for flag, want := range map[string]string{"-U": "auth_owner", "-d": "auth", "-h": "127.0.0.1", "-v": "ON_ERROR_STOP=1"} {
		if got := value(flag); got != want {
			t.Fatalf("updater auth session %s = %q, want %q", flag, got, want)
		}
	}
	if !slices.Contains(flags, "-XqAt") || !slices.Contains(flags, "--no-password") {
		t.Fatalf("updater auth session flags: %q", flags)
	}
	return updaterAuthRead{sql: value("-c"), options: strings.TrimPrefix(args[program-1], "PGOPTIONS=")}
}

func TestAuthRead_PlantedRelationsRunAsTheOwnerRole(t *testing.T) {
	fixture := newPlantedAuth(t)
	fixture.plant(t, `
CREATE VIEW public.goose_db_version AS SELECT public.planted_attempt() AS version_id;
CREATE VIEW public.corpus_release AS
  SELECT CASE WHEN public.planted_attempt() = 42 THEN 'corpus_vn_20990101' END AS active_db, 'vn'::text AS country;
`)
	version, active := authReadFromUpdater(t, "AUTH_SCHEMA_VERSION"), authReadFromUpdater(t, "AUTH_ACTIVE_DATABASE")
	if got := frameworksPsql(t, fixture.psql, fixture.owner, version.options, "", "-c", version.sql); got != "42" {
		t.Fatalf("schema read = %q", got)
	}
	if got := frameworksPsql(t, fixture.psql, fixture.owner, active.options, "", "-c", active.sql); got != "corpus_vn_20990101" {
		t.Fatalf("active database read = %q", got)
	}
	entries := fixture.log(t)
	if len(entries) < 2 {
		t.Fatalf("planted code did not run on both reads: %v", entries)
	}
	for _, entry := range entries {
		if entry.who != fixture.ownerRole || entry.outcome != "denied" {
			t.Fatalf("planted code ran as %q with outcome %q", entry.who, entry.outcome)
		}
	}
	if fixture.victimIsSuperuser(t) {
		t.Fatal("planted code gained superuser through the owner role")
	}
	frameworksPsql(t, fixture.psql, fixture.admin, version.options, "", "-c", version.sql)
	entries = fixture.log(t)
	last := entries[len(entries)-1]
	if last.who != fixture.admin.User || last.outcome != "escalated" || !fixture.victimIsSuperuser(t) {
		t.Fatalf("control: the same read as a superuser must run the plant with its rights, got %+v", last)
	}
}

type provisionerRun struct {
	script, password string
	env              []string
}

func prepareProvisioner(t *testing.T, fixture *plantedAuth) provisionerRun {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(repositoryRoot(t), "deploy/host/provision-activator.sh"))
	if err != nil {
		t.Fatal(err)
	}
	work := t.TempDir()
	password := plantedName(t, "synthetic_")
	secrets := map[string]string{"activator": password, "auth": fixture.owner.Password}
	for name, value := range secrets {
		if err = os.WriteFile(filepath.Join(work, name), []byte(value+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	wrapper := filepath.Join(work, "postgres-admin")
	if err = os.WriteFile(wrapper, []byte("#!/bin/sh\nexport PGPASSWORD=\"$TEST_ADMIN_PASSWORD\" PGHOST=\"$TEST_HOST\"\nexec \"$@\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	port := strconv.Itoa(int(fixture.admin.Port))
	text := strings.NewReplacer(
		"/run/secrets/activator_db_password", filepath.Join(work, "activator"),
		"/run/secrets/auth_db_password", filepath.Join(work, "auth"),
		"/usr/local/bin/postgres-admin.sh", wrapper,
		"--username postgres", "--username "+fixture.admin.User,
		"--username auth_owner", "--username "+fixture.ownerRole,
		"--dbname auth", "--dbname "+fixture.admin.Database,
		"--host 127.0.0.1", "--host "+fixture.admin.Host,
		"ON DATABASE auth", "ON DATABASE "+pgx.Identifier{fixture.admin.Database}.Sanitize(),
		"activator", fixture.provisioned,
	).Replace(string(raw))
	script := filepath.Join(work, "provision-activator.sh")
	if err = os.WriteFile(script, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	env := append(os.Environ(), "TEST_ADMIN_PASSWORD="+fixture.admin.Password, "TEST_HOST="+fixture.admin.Host, "PGPORT="+port)
	if filepath.IsAbs(fixture.psql) {
		env = append(env, "PATH="+filepath.Dir(fixture.psql)+string(os.PathListSeparator)+os.Getenv("PATH"))
	}
	return provisionerRun{script: script, password: password, env: env}
}

func (r provisionerRun) run(t *testing.T, overStdin bool) string {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "bash", r.script)
	if overStdin {
		raw, err := os.ReadFile(r.script)
		if err != nil {
			t.Fatal(err)
		}
		cmd = exec.CommandContext(t.Context(), "bash", "-s")
		cmd.Stdin = bytes.NewReader(raw)
	}
	cmd.Env = r.env
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("provisioner failed: %v: %.600s", err, stderr.String())
	}
	return stderr.String()
}

func TestProvisioner_StaysOutOfAuthAsSuperuserAndIgnoresItsDefaults(t *testing.T) {
	fixture := newPlantedAuth(t)
	ctx := t.Context()
	role, database := fixture.provisioned, pgx.Identifier{fixture.admin.Database}.Sanitize()
	fixture.plant(t, `
CREATE TABLE public.corpus_release (country text);
CREATE TABLE public.release_journal (id bigserial);
CREATE TABLE public.retired_release (db_name text);
CREATE TABLE public.frameworks_release (country text);
CREATE FUNCTION public.format(text, text) RETURNS text LANGUAGE plpgsql AS $plant$
BEGIN
  PERFORM public.planted_attempt();
  RETURN 'SELECT 1';
END
$plant$;
ALTER DATABASE `+database+` SET debug_print_parse = on;
ALTER DATABASE `+database+` SET client_min_messages = log;
ALTER DATABASE `+database+` SET password_encryption = 'md5';
`)
	if _, err := fixture.auth.Exec(ctx, `
CREATE TABLE public.session_log (who text NOT NULL);
CREATE FUNCTION public.record_login() RETURNS event_trigger LANGUAGE plpgsql SECURITY DEFINER SET search_path = pg_catalog AS $login$
BEGIN
  INSERT INTO public.session_log (who) VALUES (session_user);
END
$login$;
CREATE EVENT TRIGGER record_login ON login EXECUTE FUNCTION public.record_login();
`); err != nil {
		t.Fatalf("install login observer: %v", err)
	}
	run := prepareProvisioner(t, fixture)
	for _, overStdin := range []bool{true, false} {
		stderr := run.run(t, overStdin)
		if !strings.Contains(stderr, "parse tree") {
			t.Fatal("control: the planted debug_print_parse default must reach the session in auth")
		}
		if strings.Contains(stderr, run.password) {
			t.Fatal("provisioner exposed the activator password through planted session defaults")
		}
	}
	rows, err := fixture.auth.Query(ctx, `SELECT DISTINCT who FROM public.session_log ORDER BY who`)
	if err != nil {
		t.Fatal(err)
	}
	who, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(who, []string{fixture.ownerRole}) {
		t.Fatalf("sessions opened in auth: %q, want only the owner role", who)
	}
	if entries := fixture.log(t); len(entries) != 0 || fixture.victimIsSuperuser(t) {
		t.Fatalf("provisioner ran planted code: %v", entries)
	}
	var scram, granted bool
	if err = fixture.auth.QueryRow(ctx, `SELECT rolpassword LIKE 'SCRAM-SHA-256$%' FROM pg_catalog.pg_authid WHERE rolname=$1`, role).Scan(&scram); err != nil {
		t.Fatal(err)
	}
	if !scram {
		t.Fatal("planted password_encryption changed the stored activator password")
	}
	if err = fixture.auth.QueryRow(ctx, `SELECT has_database_privilege($1,current_database(),'CONNECT')
		AND has_table_privilege($1,'public.corpus_release','SELECT')
		AND has_table_privilege($1,'public.frameworks_release','INSERT')
		AND has_sequence_privilege($1,'public.release_journal_id_seq','USAGE')`, role).Scan(&granted); err != nil {
		t.Fatal(err)
	}
	if !granted {
		t.Fatal("provisioner did not apply its grants")
	}
	frameworksPsql(t, fixture.psql, fixture.admin, "-c search_path=pg_catalog,public", "SELECT format('%L', 'synthetic');\n")
	entries := fixture.log(t)
	if len(entries) != 1 || entries[0].who != fixture.admin.User || entries[0].outcome != "escalated" || !fixture.victimIsSuperuser(t) {
		t.Fatalf("control: a superuser session in auth must reach the plant, got %v", entries)
	}
}
