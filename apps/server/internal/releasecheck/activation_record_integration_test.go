//go:build integration

package releasecheck

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/server/internal/migrate"
	"danny.vn/trachuan/internal/testpg"
	"github.com/jackc/pgx/v5"
)

const phaseHarness = `import hashlib
import json
import os
from pathlib import Path
import subprocess
import sys
import time

root = Path(sys.argv[1])
sys.path.insert(0, str(root / 'deploy/host'))

from updater import deploy_corpus_lock
from updater.deploy_activation_record import stamp
from updater.deploy_corpus import Corpus
from updater.deploy_corpus_database import Database
from updater.deploy_corpus_kind import FRAMEWORKS, LAW
from updater.deploy_host import Host
from updater.deploy_io import canonical, command
from updater.deploy_state import State
from updater.deploy_store import Paths


class Crash(BaseException):
    pass


c = json.load(sys.stdin)
kind = FRAMEWORKS if c['kind'] == 'frameworks' else LAW
work = Path(c['work'])
paths = Paths(work / 'root')
for path in (paths.state, paths.host, paths.result.parent, paths.log.parent, paths.releases):
    path.mkdir(parents=True, exist_ok=True)
ROLES = {'activator': (c['activator'], c['activator_password']), 'auth_owner': (c['owner'], c['owner_password'])}


def client(args):
    out, role = [], None
    index = 0
    while index < len(args):
        arg = args[index]
        if arg == '-h':
            out += ['-h', c['host'], '-p', str(c['port'])]
            index += 2
        elif arg == '-U':
            role = ROLES[args[index + 1]]
            out += ['-U', role[0]]
            index += 2
        elif arg in ('-d', '--dbname') and args[index + 1] == 'auth':
            out += [arg, c['auth']]
            index += 2
        else:
            out.append(os.path.join(c['bin'], arg) if arg in ('psql', 'createdb', 'dropdb') else arg)
            index += 1
    return ['env', 'PGPASSWORD=' + role[1]] + out, role[0]


def runner(args, input=None, **options):
    if args[:2] == ['podman', 'exec'] and ('rm' in args or 'rm -f /run/pgbackrest/corpus-*.list' in args):
        return b''
    if args[:6] != ['podman', 'exec', '-i', 'trachuan-postgres', 'sh', '-c']:
        raise RuntimeError('unexpected host command')
    command_line, role = client(args[8:])
    allowed = {key: options[key] for key in ('timeout', 'stdout_limit') if key in options}
    return command(command_line, input=input, **allowed).replace(c['activator'].encode(), b'activator')


real_popen = subprocess.Popen
def popen(args, **options):
    return real_popen(client(args[8:])[0], **options)
deploy_corpus_lock.subprocess = type('S', (), dict(Popen=staticmethod(popen), PIPE=subprocess.PIPE,
                                                   DEVNULL=subprocess.DEVNULL, TimeoutExpired=subprocess.TimeoutExpired))


def write_artifact(manifest):
    raw = canonical(manifest)
    digest = hashlib.sha256(raw).hexdigest()
    directory = paths.state / kind.artifacts / digest
    directory.mkdir(parents=True, exist_ok=True)
    (directory / 'manifest.json').write_bytes(raw)
    (directory / 'manifest.sigstore.json').write_bytes(b'{}')
    return digest, directory


state_path = paths.state / 'state.json'
if not state_path.exists():
    State.bootstrap(state_path, 'f' * 64, 1, 7, 'e' * 64)
state = State(state_path)
previous_hash, _ = write_artifact(c['previous'])
records = state.section()['accepted'] if kind is FRAMEWORKS else state.data['accepted']['corpus']
if not records:
    records['1'] = dict(hash=previous_hash, status='active')
    state.save()

digest, directory = write_artifact(c['manifest'])
kind.verify = lambda path, rules, run: (digest, c['manifest'])
kind.compatible = lambda manifest, report: True


class Registry:
    def fetch(self, repository, tag, target, label):
        return directory


class Steps:
    def __init__(self, db):
        self.db = db

    def step(self, name):
        if c.get('crash') == name:
            raise Crash(name)

    def inspect(self, row, dump=None, image=None):
        self.step('inspect')

    def create(self, row):
        self.step('create')
        self.db.make(row)

    def restore(self, row, dump=None, image=None):
        self.step('restore')

    def validate(self, row):
        self.step('validate')

    def smoke(self, row, image):
        self.step('smoke')


rules = dict(corpus_repository='x', release_namespace='n', release_bucket='b', region='r',
             extensions=['vector', 'ltree'], frameworks=dict(repository='x', namespace='n', bucket='b', min_release_floor=0))
host = Host(paths, runner, user_prefix=[])
database = Database(host, rules, kind)
database.describe = lambda image, name=None, **options: {}
database.check_extensions = lambda manifest: None
database.other_superuser = lambda: False
phase = Corpus(state, None, host, rules, Registry(), database=database, kind=kind)
phase.provision = lambda: None
phase.resources = lambda: None
phase.image = lambda: 'image'
phase.download = lambda: paths.releases / 'unused'
phase.engine.steps = Steps(database)
if c.get('age_retired'):
    record = phase.records.own
    if record.load() == 'ok':
        for entry in record.retired:
            entry['retired_at'] = stamp(time.time() - 8 * 86400)
        record.save()
    (paths.state / kind.marker).unlink(missing_ok=True)
crashed = False
try:
    outcome = phase.run(work / 'run', c.get('mode', 'activate'))
except Crash:
    crashed, outcome = True, None
result_path = paths.result.with_name(kind.result + '.json')
record_path = paths.state / (kind.key + '-activation.json')
print(json.dumps(dict(outcome=outcome, crashed=crashed,
                      result=json.loads(result_path.read_bytes()) if result_path.exists() else None,
                      record=json.loads(record_path.read_bytes()) if record_path.exists() else None,
                      statuses={r['hash']: r['status'] for r in State(state_path).records(kind.key).values()},
                      digest=digest)))
`

type phaseOutcome struct {
	Outcome *bool  `json:"outcome"`
	Crashed bool   `json:"crashed"`
	Digest  string `json:"digest"`
	Result  struct {
		Status    string   `json:"status"`
		Message   string   `json:"message"`
		Mode      *string  `json:"mode"`
		Integrity []string `json:"integrity"`
	} `json:"result"`
	Record struct {
		Active *struct {
			Database string `json:"database"`
			OID      int64  `json:"oid"`
		} `json:"active"`
		Retired []struct {
			Database string `json:"database"`
		} `json:"retired"`
		Operation *struct {
			Step string `json:"step"`
		} `json:"operation"`
	} `json:"record"`
	Statuses map[string]string `json:"statuses"`
}

type phaseFixture struct {
	planted  *plantedAuth
	admin    *pgx.Conn
	psql     string
	password string
	kind     string
	prefix   string
	table    string
	readable string
	work     string
	rels     []string
	previous map[string]any
	marked   map[string]string
}

func newPhaseFixture(t *testing.T, kind string, rels []string, flags ...string) *phaseFixture {
	t.Helper()
	ctx := t.Context()
	planted := newPlantedAuth(t)
	if err := migrate.Up(ctx, planted.authURL); err != nil {
		t.Fatal(err)
	}
	database := pgx.Identifier{planted.admin.Database}.Sanitize()
	owner := pgx.Identifier{planted.ownerRole}.Sanitize()
	if _, err := planted.auth.Exec(ctx, "ALTER DATABASE "+database+" OWNER TO "+owner); err != nil {
		t.Fatal(err)
	}
	for _, table := range releaseTables {
		if _, err := planted.auth.Exec(ctx, "ALTER TABLE public."+table+" OWNER TO "+owner); err != nil {
			t.Fatal(err)
		}
	}
	admin, _, err := testpg.OpenAdministrator(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Close(context.Background()) })
	provisioner := prepareProvisioner(t, planted)
	provisioner.run(t, false, flags...)
	fixture := &phaseFixture{planted: planted, admin: admin, psql: planted.psql, password: provisioner.password, kind: kind,
		prefix: "corpus_vn_", table: "corpus_release", work: t.TempDir(), rels: rels, marked: map[string]string{}}
	if kind == "frameworks" {
		fixture.prefix, fixture.table = "frameworks_", "frameworks_release"
	}
	fixture.readable = fixture.table
	first := fixture.name(0)
	fixture.create(t, first)
	fixture.previous = fixture.manifest(0, "", 1, "a")
	if _, err = planted.auth.Exec(ctx, "INSERT INTO public."+fixture.table+" (country,active_db,activated_at,activated_by) VALUES ('vn',$1,now(),'fixture')", first); err != nil {
		t.Fatal(err)
	}
	return fixture
}

var releaseTables = []string{"corpus_release", "frameworks_release", "release_journal", "retired_release"}

func (f *phaseFixture) name(index int) string { return f.prefix + f.rels[index] }

func (f *phaseFixture) mark(t *testing.T, name string) {
	t.Helper()
	key := "vn/" + strings.TrimPrefix(strings.TrimPrefix(name, "corpus_vn_"), "frameworks_")
	if _, seen := f.marked[name]; !seen {
		f.marked[name] = key
		t.Cleanup(func() { cleanupReleaseDatabase(t, f.admin, name, key) })
	}
	var exists bool
	if err := f.admin.QueryRow(t.Context(), `SELECT EXISTS(SELECT FROM pg_database WHERE datname=$1)`, name).Scan(&exists); err != nil {
		t.Fatal(err)
	}
	if exists {
		if _, err := f.admin.Exec(t.Context(), "COMMENT ON DATABASE "+pgx.Identifier{name}.Sanitize()+` IS '{"run_key":"`+key+`"}'`); err != nil {
			t.Fatal(err)
		}
	}
}

func (f *phaseFixture) markReleases(t *testing.T) {
	t.Helper()
	for index := range f.rels {
		f.mark(t, f.name(index))
	}
}

func (f *phaseFixture) create(t *testing.T, name string) {
	t.Helper()
	if _, err := f.admin.Exec(t.Context(), "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()+" OWNER "+pgx.Identifier{f.planted.provisioned}.Sanitize()+" TEMPLATE template0"); err != nil {
		t.Fatal(err)
	}
	f.mark(t, name)
}

func (f *phaseFixture) manifest(index int, previous string, sequence int, digest string) map[string]any {
	manifest := map[string]any{"database": f.name(index), "rel": f.rels[index], "sequence": sequence,
		"dump": map[string]any{"sha256": strings.Repeat(digest, 64), "size": 1}, "extensions": map[string]any{}, "schema_version": 1}
	if f.kind == "law" {
		manifest["country"] = "vn"
	}
	if previous != "" {
		manifest["previous"] = previous
	} else if f.kind == "frameworks" {
		manifest["previous"] = nil
	}
	return manifest
}

func (f *phaseFixture) run(t *testing.T, manifest map[string]any, options map[string]any) phaseOutcome {
	t.Helper()
	config := f.planted.admin
	bin := ""
	if filepath.IsAbs(f.psql) {
		bin = filepath.Dir(f.psql)
	}
	input := map[string]any{"kind": f.kind, "work": f.work, "manifest": manifest, "previous": f.previous,
		"activator": f.planted.provisioned, "activator_password": f.password, "owner": f.planted.ownerRole,
		"owner_password": f.planted.owner.Password, "host": config.Host, "port": config.Port,
		"auth": config.Database, "bin": bin}
	for key, value := range options {
		input[key] = value
	}
	raw, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(t.Context(), "python3", "-I", "-B", "-c", phaseHarness, repositoryRoot(t))
	cmd.Env = append(os.Environ(), "PGCONNECT_TIMEOUT=5", "PGOPTIONS=")
	cmd.Stdin = bytes.NewReader(raw)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	f.markReleases(t)
	if err != nil {
		t.Fatalf("phase harness: %v: %.1200s", err, stderr.String())
	}
	var outcome phaseOutcome
	if err = json.Unmarshal(output, &outcome); err != nil {
		t.Fatalf("decode phase harness output: %v: %.400s", err, output)
	}
	return outcome
}

func (f *phaseFixture) pointer(t *testing.T) string {
	t.Helper()
	var pointer string
	if err := f.planted.auth.QueryRow(t.Context(), "SELECT active_db FROM public."+f.readable+" WHERE country='vn'").Scan(&pointer); err != nil {
		t.Fatal(err)
	}
	return pointer
}

func (f *phaseFixture) exists(t *testing.T, name string) bool {
	t.Helper()
	var found bool
	if err := f.admin.QueryRow(t.Context(), `SELECT EXISTS(SELECT FROM pg_database WHERE datname=$1)`, name).Scan(&found); err != nil {
		t.Fatal(err)
	}
	return found
}

func wantOutcome(t *testing.T, got phaseOutcome, status string) {
	t.Helper()
	if got.Crashed || got.Result.Status != status {
		t.Fatalf("phase result = %+v; want status %s", got, status)
	}
}

const plantMark = `
CREATE FUNCTION public.planted_mark() RETURNS boolean LANGUAGE plpgsql AS $plant$
BEGIN
  IF pg_catalog.current_setting('transaction_read_only')::boolean THEN
    IF session_user <> 'OWNER_ROLE' THEN
      RAISE EXCEPTION 'planted code ran as %', session_user;
    END IF;
  ELSE
    INSERT INTO public.planted_log (who, outcome) VALUES (session_user, 'ran');
  END IF;
  RETURN true;
END
$plant$;
CREATE FUNCTION public.planted_row() RETURNS trigger LANGUAGE plpgsql AS $plant$
BEGIN
  PERFORM public.planted_mark();
  IF TG_OP = 'DELETE' THEN
    RETURN OLD;
  END IF;
  RETURN NEW;
END
$plant$;
`

func plantOnTables(t *testing.T, fixture *phaseFixture, shape string) {
	t.Helper()
	planted := fixture.planted
	planted.plant(t, strings.ReplaceAll(plantMark, "OWNER_ROLE", planted.ownerRole))
	if shape == "view" {
		fixture.readable = fixture.table + "_base"
	}
	for _, table := range releaseTables {
		var statement string
		switch shape {
		case "trigger":
			statement = fmt.Sprintf(`CREATE TRIGGER planted BEFORE INSERT OR UPDATE OR DELETE ON public.%[1]s FOR EACH ROW EXECUTE FUNCTION public.planted_row()`, table)
		case "policy":
			statement = fmt.Sprintf(`ALTER TABLE public.%[1]s ENABLE ROW LEVEL SECURITY; ALTER TABLE public.%[1]s FORCE ROW LEVEL SECURITY;
CREATE POLICY planted ON public.%[1]s USING (public.planted_mark()) WITH CHECK (public.planted_mark())`, table)
		case "view":
			statement = fmt.Sprintf(`ALTER TABLE public.%[1]s RENAME TO %[1]s_base;
CREATE VIEW public.%[1]s AS SELECT * FROM public.%[1]s_base WHERE public.planted_mark()`, table)
		}
		planted.plant(t, statement)
	}
}

func TestActivationRecord_PlantedObjectsNeverRunAsActivator(t *testing.T) {
	for _, shape := range []string{"trigger", "view", "policy"} {
		for name, flags := range map[string][]string{"revoked": {"--no-auth-tables"}, "granted": nil} {
			for _, kind := range []string{"law", "frameworks"} {
				t.Run(shape+"/"+name+"/"+kind, func(t *testing.T) {
					withLawReleases(t, 2, nil, func(t *testing.T, rels []string) {
						fixture := newPhaseFixture(t, kind, rels, flags...)
						plantOnTables(t, fixture, shape)
						got := fixture.run(t, fixture.manifest(1, rels[0], 2, "b"), nil)
						wantOutcome(t, got, "ok")
						if fixture.pointer(t) != fixture.name(1) {
							t.Fatalf("pointer = %s", fixture.pointer(t))
						}
						entries := fixture.planted.log(t)
						if len(entries) == 0 {
							t.Fatal("the planted object never ran, so the test proves nothing")
						}
						for _, entry := range entries {
							if entry.who != fixture.planted.ownerRole {
								t.Fatalf("planted code ran as %q; only the auth owner role %q may reach it", entry.who, fixture.planted.ownerRole)
							}
						}
					})
				})
			}
		}
	}
}

func TestActivationRecord_NewRetainedPruneAndCrashRecoveryWorkFromTheRecord(t *testing.T) {
	for _, kind := range []string{"law", "frameworks"} {
		t.Run(kind, func(t *testing.T) {
			withLawReleases(t, 4, nil, func(t *testing.T, rels []string) {
				fixture := newPhaseFixture(t, kind, rels, "--no-auth-tables")
				ctx := t.Context()
				got := fixture.run(t, fixture.manifest(1, rels[0], 2, "b"), nil)
				wantOutcome(t, got, "ok")
				if got.Result.Mode == nil || *got.Result.Mode != "new" || got.Record.Active == nil || got.Record.Active.Database != fixture.name(1) {
					t.Fatalf("new activation: %+v", got)
				}
				var digest *string
				if err := fixture.planted.auth.QueryRow(ctx, `SELECT dump_sha256 FROM public.retired_release WHERE db_name=$1`, fixture.name(0)).Scan(&digest); err != nil || digest == nil || *digest != strings.Repeat("a", 64) {
					t.Fatalf("retirement of the previous database: %v %v", err, digest)
				}

				retained := fixture.manifest(0, rels[1], 3, "a")
				got = fixture.run(t, retained, nil)
				wantOutcome(t, got, "ok")
				if *got.Result.Mode != "retained" || fixture.pointer(t) != fixture.name(0) {
					t.Fatalf("retained reactivation: %+v pointer %s", got, fixture.pointer(t))
				}
				var count int
				if err := fixture.planted.auth.QueryRow(ctx, `SELECT count(*) FROM public.retired_release WHERE db_name=$1`, fixture.name(0)).Scan(&count); err != nil || count != 0 {
					t.Fatalf("retained target still retired: %v %d", err, count)
				}

				if _, err := fixture.planted.auth.Exec(ctx, `UPDATE public.retired_release SET retain_until=now()-interval '1 second' WHERE db_name=$1`, fixture.name(1)); err != nil {
					t.Fatal(err)
				}
				got = fixture.run(t, retained, map[string]any{"age_retired": true})
				wantOutcome(t, got, "current")
				if fixture.exists(t, fixture.name(1)) {
					t.Fatal("prune kept the aged database")
				}
				if len(got.Record.Retired) != 0 {
					t.Fatalf("prune kept the entry: %+v", got.Record.Retired)
				}
				var pruned int
				if err := fixture.planted.auth.QueryRow(ctx, `SELECT count(*) FROM public.release_journal WHERE mode='prune' AND step='pruned' AND database_name=$1`, fixture.name(1)).Scan(&pruned); err != nil || pruned != 1 {
					t.Fatalf("prune journal: %v %d", err, pruned)
				}

				crashing := fixture.manifest(2, rels[0], 4, "c")
				got = fixture.run(t, crashing, map[string]any{"crash": "validate"})
				if !got.Crashed || got.Record.Operation == nil || got.Record.Operation.Step != "restored" {
					t.Fatalf("crash left: %+v", got)
				}
				if got.Statuses[got.Digest] != "pending" || fixture.pointer(t) != fixture.name(0) {
					t.Fatalf("crash changed state: %+v", got.Statuses)
				}
				got = fixture.run(t, crashing, nil)
				wantOutcome(t, got, "ok")
				if got.Record.Operation != nil || fixture.pointer(t) != fixture.name(2) || got.Statuses[got.Digest] != "active" {
					t.Fatalf("recovery: %+v", got)
				}
			})
		})
	}
}

func TestActivationRecord_TableFailureWaitsWithoutSpendingTheManifest(t *testing.T) {
	withLawReleases(t, 2, nil, func(t *testing.T, rels []string) {
		fixture := newPhaseFixture(t, "law", rels, "--no-auth-tables")
		ctx := t.Context()
		if _, err := fixture.planted.auth.Exec(ctx, `ALTER TABLE public.release_journal RENAME TO release_journal_moved`); err != nil {
			t.Fatal(err)
		}
		manifest := fixture.manifest(1, rels[0], 2, "b")
		got := fixture.run(t, manifest, nil)
		wantOutcome(t, got, "waiting")
		if got.Statuses[got.Digest] == "failed" || fixture.pointer(t) != fixture.name(0) {
			t.Fatalf("table failure spent the manifest: %+v", got)
		}
		if _, err := fixture.planted.auth.Exec(ctx, `ALTER TABLE public.release_journal_moved RENAME TO release_journal`); err != nil {
			t.Fatal(err)
		}
		wantOutcome(t, fixture.run(t, manifest, nil), "ok")
		if fixture.pointer(t) != fixture.name(1) {
			t.Fatal("the release did not activate after the table returned")
		}
	})
}

func TestActivationRecord_AdvisoryLockExcludesAConcurrentRunAcrossCodeShapes(t *testing.T) {
	withLawReleases(t, 2, nil, func(t *testing.T, rels []string) {
		fixture := newPhaseFixture(t, "law", rels, "--no-auth-tables")
		ctx := t.Context()
		config := fixture.planted.admin.Copy()
		config.User, config.Password = fixture.planted.provisioned, fixture.password
		holder, err := pgx.ConnectConfig(ctx, config)
		if err != nil {
			t.Fatal("connect lock holder")
		}
		defer func() { _ = holder.Close(context.Background()) }()
		var held bool
		if err = holder.QueryRow(ctx, `SELECT pg_try_advisory_lock(hashtextextended('activation',0))`).Scan(&held); err != nil || !held {
			t.Fatalf("older code shape did not take the lock: %v %t", err, held)
		}
		manifest := fixture.manifest(1, rels[0], 2, "b")
		got := fixture.run(t, manifest, nil)
		wantOutcome(t, got, "skipped")
		if !strings.Contains(got.Result.Message, "advisory lock busy") || fixture.pointer(t) != fixture.name(0) {
			t.Fatalf("busy lock: %+v", got)
		}
		if _, err = holder.Exec(ctx, `SELECT pg_advisory_unlock(hashtextextended('activation',0))`); err != nil {
			t.Fatal(err)
		}
		wantOutcome(t, fixture.run(t, manifest, nil), "ok")
	})
}
