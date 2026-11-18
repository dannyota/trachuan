//go:build integration

package releasecheck

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/server/internal/migrate"
	"danny.vn/trachuan/internal/testpg"
	"github.com/jackc/pgx/v5"
)

const lawActivationBridge = `import json,os,sys
from pathlib import Path
sys.path.insert(0,sys.argv[1])
from updater.deploy_corpus_activate import Activation
from updater.deploy_corpus_database import Database
from updater.deploy_corpus_kind import FRAMEWORKS, LAW
from updater.deploy_host import Host
from updater.deploy_io import command
from updater.deploy_release_target import Leftover
from updater.deploy_store import Paths
c=json.load(sys.stdin)
def translate(cmd):
    out,i=[],0
    while i<len(cmd):
        a=cmd[i]
        if a=='-h':
            out+=['-h',c['host'],'-p',str(c['port'])]; i+=2
        elif a=='-U':
            out+=['-U',c['role']]; i+=2
        elif a=='-d' and cmd[i+1]=='auth':
            out+=['-d',c['auth']]; i+=2
        else:
            out.append(os.path.join(c['bin'],a) if c['bin'] and a in ('psql','createdb','dropdb') else a); i+=1
    return out
def runner(args,input=None,**kwargs):
    if '/usr/local/bin/postgres-admin.sh' in args:
        raise AssertionError('postgres session opened')
    if args[4:6]!=['sh','-c']:
        return b''
    return command(translate(args[8:]),input=input).replace(c['role'].encode(),b'activator')
class Held:
    def poll(self):
        return None
kind=FRAMEWORKS if c['kind']=='frameworks' else LAW
db=Database(Host(Paths(Path(c['root'])),runner,user_prefix=[]),{'extensions':['vector','ltree']},kind)
db.holder=Held()
db.backend=c.get('backend',0)
engine=Activation(db,kind)
actions={'choose':lambda:engine.choose(c['manifest'])[0],'make':lambda:db.make(c['row']) or 'ok','abort':lambda:engine.abort(c['row']) or 'ok',
 'verify':lambda:db.verify_target(c['row']) or 'ok','oid':lambda:db.target_oid(c['row'])}
results=[]
for name in c['actions']:
    try:
        results.append(actions[name]())
    except ValueError as error:
        results.append('refused: '+str(error))
    except Leftover as error:
        results.append('waiting: '+str(error))
print(json.dumps(results))
`

func lawActivationSteps(t *testing.T, config *pgx.ConnConfig, psql string, request map[string]any, actions ...string) []string {
	t.Helper()
	bin := ""
	if filepath.IsAbs(psql) {
		bin = filepath.Dir(psql)
	}
	input := map[string]any{"host": config.Host, "port": config.Port, "role": config.User, "bin": bin, "actions": actions}
	for key, value := range request {
		input[key] = value
	}
	raw, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(t.Context(), "python3", "-I", "-B", "-c", lawActivationBridge, filepath.Join(repositoryRoot(t), "deploy/host"))
	cmd.Env = append(os.Environ(), "PGPASSWORD="+config.Password, "PGCONNECT_TIMEOUT=5")
	cmd.Stdin = bytes.NewReader(raw)
	output, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			t.Fatalf("activation steps failed: %v: %.600s", err, bytes.TrimSpace(exitErr.Stderr))
		}
		t.Fatalf("activation steps failed: %v", err)
	}
	var results []string
	if err = json.Unmarshal(output, &results); err != nil {
		t.Fatal("decode activation step results")
	}
	return results
}

func lawTargetAuth(t *testing.T, activator string) *pgx.Conn {
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
	role := pgx.Identifier{activator}.Sanitize()
	if _, err = auth.Exec(ctx, "GRANT USAGE ON SCHEMA public TO "+role+"; GRANT SELECT,INSERT,UPDATE,DELETE ON corpus_release,release_journal,retired_release TO "+role+"; GRANT USAGE,SELECT ON SEQUENCE release_journal_id_seq TO "+role+"; GRANT SELECT,INSERT,UPDATE ON frameworks_release TO "+role); err != nil {
		t.Fatal(err)
	}
	return auth
}

func withLawReleases(t *testing.T, count int, found []string, body func(*testing.T, []string)) {
	t.Helper()
	found = append(found, testpg.Release(t))
	if len(found) == count {
		body(t, found)
		return
	}
	t.Run("next", func(t *testing.T) { withLawReleases(t, count, found, body) })
}

func TestLawRestore_CreateEndsBackendsDropsStraysAndPinsTheTarget(t *testing.T) {
	_, _, psqlPath := frameworksClients(t)
	withLawReleases(t, 7, nil, func(t *testing.T, rels []string) {
		ctx := t.Context()
		admin, _, err := testpg.OpenAdministrator(ctx)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = admin.Close(context.Background()) })
		activator := testpg.NewBuilder(t)
		role := pgx.Identifier{activator.Role}.Sanitize()
		target, runKey := "corpus_vn_"+rels[0], "vn/"+rels[0]
		lawActive, lawRetired, lawExpired := "corpus_vn_"+rels[1], "corpus_vn_"+rels[2], "corpus_vn_"+rels[3]
		frameworksActive, frameworksRetired, frameworksOpen := "frameworks_"+rels[4], "frameworks_"+rels[5], "frameworks_"+rels[6]
		spare := "spare_" + strings.ReplaceAll(rels[0], "_", "")
		moved, replacement := "moved_"+strings.ReplaceAll(rels[0], "_", ""), "replacement_"+strings.ReplaceAll(rels[0], "_", "")
		mark := func(name, key string) {
			t.Helper()
			if _, markErr := admin.Exec(ctx, "COMMENT ON DATABASE "+pgx.Identifier{name}.Sanitize()+` IS '{"run_key":"`+key+`"}'`); markErr != nil {
				t.Fatal(markErr)
			}
		}
		for _, item := range []struct{ name, key string }{
			{lawActive, "vn/" + rels[1]}, {lawRetired, "vn/" + rels[2]}, {lawExpired, "vn/" + rels[3]},
			{frameworksActive, "vn/" + rels[4]}, {frameworksRetired, "vn/" + rels[5]}, {frameworksOpen, "vn/" + rels[6]}, {spare, runKey},
		} {
			if _, err = admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{item.name}.Sanitize()+" OWNER "+role+" TEMPLATE template0"); err != nil {
				t.Fatal(err)
			}
			mark(item.name, item.key)
			t.Cleanup(func() { testpg.DropCorpusDatabase(t, admin, item.name, item.key) })
		}
		t.Cleanup(func() { testpg.DropCorpusDatabase(t, admin, target, runKey) })
		t.Cleanup(func() { testpg.DropCorpusDatabase(t, admin, moved, runKey) })

		auth := lawTargetAuth(t, activator.Role)
		digest := strings.Repeat("d", 64)
		for _, statement := range []struct {
			sql  string
			args []any
		}{
			{`INSERT INTO corpus_release VALUES ('vn',$1,now(),'fixture')`, []any{lawActive}},
			{`INSERT INTO frameworks_release VALUES ('vn',$1,now(),'fixture')`, []any{frameworksActive}},
			{`INSERT INTO retired_release (country,db_name,retired_at,retain_until,dump_sha256) VALUES ('vn',$1,now(),now()+interval '7 days',$2)`, []any{lawRetired, digest}},
			{`INSERT INTO retired_release (country,db_name,retired_at,retain_until,dump_sha256) VALUES ('vn',$1,now()-interval '8 days',now()-interval '1 day',$2)`, []any{lawExpired, digest}},
			{`INSERT INTO retired_release (country,db_name,retired_at,retain_until,dump_sha256,corpus_kind) VALUES ('vn',$1,now(),now()+interval '7 days',$2,'frameworks')`, []any{frameworksRetired, digest}},
		} {
			if _, err = auth.Exec(ctx, statement.sql, statement.args...); err != nil {
				t.Fatal(err)
			}
		}
		journal := func(operation, rel, name string, expected any) map[string]any {
			return map[string]any{"operation_id": operation, "country": "vn", "rel": rel, "mode": "new", "database_name": name, "manifest_hash": strings.Repeat("c", 64), "dump_sha256": digest, "expected_active_db": expected, "manifest": map[string]any{"fixture": true}, "step": "creating"}
		}
		open, refused := frameworksUpdater(t, "sql", "journal", journal(strings.Repeat("f", 32), rels[6], frameworksOpen, frameworksActive), "creating")
		if refused != "" {
			t.Fatal(refused)
		}
		row := journal(strings.Repeat("a", 32), rels[0], target, lawActive)
		for _, statement := range []string{open, pythonSQL(t, "journal", []any{row, "creating"})} {
			if _, err = auth.Exec(ctx, statement); err != nil {
				t.Fatal(err)
			}
		}

		activatorConfig, err := pgx.ParseConfig(activator.URL)
		if err != nil {
			t.Fatal("parse synthetic activator configuration")
		}
		lingerConfig := activatorConfig.Copy()
		lingerConfig.Database = spare
		linger, err := pgx.ConnectConfig(ctx, lingerConfig)
		if err != nil {
			t.Fatal("connect lingering backend")
		}
		t.Cleanup(func() { _ = linger.Close(context.Background()) })
		lockConfig := activatorConfig.Copy()
		lockConfig.Database = auth.Config().Database
		lock, err := pgx.ConnectConfig(ctx, lockConfig)
		if err != nil {
			t.Fatal("connect lock session")
		}
		t.Cleanup(func() { _ = lock.Close(context.Background()) })
		var backend int
		if err = lock.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&backend); err != nil {
			t.Fatal(err)
		}
		request := map[string]any{"kind": "law", "auth": auth.Config().Database, "row": row, "root": t.TempDir(), "backend": backend}
		blockerConfig := admin.Config().Copy()
		blockerConfig.Database = spare
		blocker, err := pgx.ConnectConfig(ctx, blockerConfig)
		if err != nil {
			t.Fatal("connect another role to the stray")
		}
		t.Cleanup(func() { _ = blocker.Close(context.Background()) })
		if got := lawActivationSteps(t, activatorConfig, psqlPath, request, "make"); !reflect.DeepEqual(got, []string{"waiting: leftover database in use"}) {
			t.Fatalf("stray in use: %q", got)
		}
		if err = blocker.Close(ctx); err != nil {
			t.Fatal(err)
		}
		if got := lawActivationSteps(t, activatorConfig, psqlPath, request, "make"); !reflect.DeepEqual(got, []string{"ok"}) {
			t.Fatalf("create step: %q", got)
		}
		mark(target, runKey)
		if linger.Ping(ctx) == nil {
			t.Fatal("lingering activator backend survived")
		}
		if err = lock.Ping(ctx); err != nil {
			t.Fatalf("lock session was ended: %v", err)
		}
		exists := func(name string) bool {
			t.Helper()
			var found bool
			if queryErr := admin.QueryRow(ctx, `SELECT EXISTS(SELECT FROM pg_database WHERE datname=$1)`, name).Scan(&found); queryErr != nil {
				t.Fatal(queryErr)
			}
			return found
		}
		for _, name := range []string{spare, lawExpired} {
			if exists(name) {
				t.Fatalf("unprotected database %s survived", name)
			}
		}
		for _, name := range []string{target, lawActive, lawRetired, frameworksActive, frameworksRetired, frameworksOpen} {
			if !exists(name) {
				t.Fatalf("protected database %s was dropped", name)
			}
		}

		var current string
		if err = admin.QueryRow(ctx, `SELECT oid::text FROM pg_database WHERE datname=$1`, target).Scan(&current); err != nil {
			t.Fatal(err)
		}
		if got := lawActivationSteps(t, activatorConfig, psqlPath, request, "oid", "verify"); !reflect.DeepEqual(got, []string{current, "ok"}) {
			t.Fatalf("recorded target identity: %q, database %q", got, current)
		}
		for _, statement := range []string{
			"ALTER DATABASE " + pgx.Identifier{target}.Sanitize() + " RENAME TO " + pgx.Identifier{moved}.Sanitize(),
			"CREATE DATABASE " + pgx.Identifier{replacement}.Sanitize() + " TEMPLATE template0",
			"ALTER DATABASE " + pgx.Identifier{replacement}.Sanitize() + " RENAME TO " + pgx.Identifier{target}.Sanitize(),
		} {
			if _, err = lock.Exec(ctx, statement); err != nil {
				t.Fatal(err)
			}
		}
		mark(target, runKey)
		want := []string{"refused: " + lawSQL(t, "target", "REFUSED")}
		if got := lawActivationSteps(t, activatorConfig, psqlPath, request, "verify"); !reflect.DeepEqual(got, want) {
			t.Fatalf("renamed-in target: got %q, want %q", got, want)
		}
		guarded := lawSQL(t, "target", "guard", current, "SELECT 'ran';")
		adminTarget := admin.Config().Copy()
		adminTarget.Database = target
		if got := frameworksPsql(t, psqlPath, adminTarget, lawSQL(t, "sql", "CATALOG_PATH"), guarded); got != lawSQL(t, "target", "MISMATCH") {
			t.Fatalf("postgres step on a renamed-in target: %q", got)
		}
		adminTarget.Database = moved
		if got := frameworksPsql(t, psqlPath, adminTarget, lawSQL(t, "sql", "CATALOG_PATH"), guarded); got != "ran" {
			t.Fatalf("guard on the recorded database: %q", got)
		}
	})
}

func TestLawRestore_TargetOwnedByAnotherRoleIsRefusedAndKept(t *testing.T) {
	_, _, psqlPath := frameworksClients(t)
	for _, tc := range []struct{ kind, label, prefix, previous, active string }{
		{"law", "corpus", "corpus_vn_", "20990101_1", "corpus_vn_20990101_1"},
		{"frameworks", "frameworks", "frameworks_", "", ""},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			ctx := t.Context()
			admin, _, err := testpg.OpenAdministrator(ctx)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = admin.Close(context.Background()) })
			activator := testpg.NewBuilder(t)
			rel := testpg.Release(t)
			name, runKey := tc.prefix+rel, "vn/"+rel
			target := pgx.Identifier{name}.Sanitize()
			mark := func() {
				t.Helper()
				if _, markErr := admin.Exec(ctx, "COMMENT ON DATABASE "+target+` IS '{"run_key":"`+runKey+`"}'`); markErr != nil {
					t.Fatal(markErr)
				}
			}
			if _, err = admin.Exec(ctx, "CREATE DATABASE "+target+" TEMPLATE template0"); err != nil {
				t.Fatal(err)
			}
			mark()
			t.Cleanup(func() { testpg.DropCorpusDatabase(t, admin, name, runKey) })
			adminTarget := admin.Config().Copy()
			adminTarget.Database = name
			frameworksPsql(t, psqlPath, adminTarget, "", "CREATE TABLE hand_restored (id integer);\n")

			auth := lawTargetAuth(t, activator.Role)
			role := pgx.Identifier{activator.Role}.Sanitize()
			manifest := map[string]any{"database": name, "country": "vn", "rel": rel, "previous": nil, "dump": map[string]any{"sha256": strings.Repeat("d", 64)}}
			var expected any
			if tc.active != "" {
				if _, err = auth.Exec(ctx, `INSERT INTO corpus_release VALUES ('vn',$1,now(),'fixture')`, tc.active); err != nil {
					t.Fatal(err)
				}
				manifest["previous"], expected = tc.previous, tc.active
			}
			row := map[string]any{"operation_id": strings.Repeat("a", 32), "country": "vn", "rel": rel, "mode": "new", "database_name": name, "manifest_hash": strings.Repeat("c", 64), "dump_sha256": strings.Repeat("d", 64), "expected_active_db": expected, "manifest": map[string]any{"fixture": true}, "step": "creating"}
			activatorConfig, err := pgx.ParseConfig(activator.URL)
			if err != nil {
				t.Fatal("parse synthetic activator configuration")
			}
			request := map[string]any{"kind": tc.kind, "auth": auth.Config().Database, "manifest": manifest, "row": row, "root": t.TempDir()}

			got := lawActivationSteps(t, activatorConfig, psqlPath, request, "choose", "make", "abort")
			want := []string{"refused: new " + tc.label + " target already exists", "refused: activation target not owned by activator", "ok"}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("target owned by another role: got %q, want %q", got, want)
			}
			var owner string
			if err = admin.QueryRow(ctx, `SELECT pg_get_userbyid(datdba)::text FROM pg_database WHERE datname=$1`, name).Scan(&owner); err != nil {
				t.Fatal(err)
			}
			if owner != admin.Config().User {
				t.Fatalf("refused target changed owner to %q", owner)
			}
			if kept := frameworksPsql(t, psqlPath, adminTarget, "", "SELECT to_regclass('public.hand_restored') IS NOT NULL;\n"); kept != "t" {
				t.Fatalf("refused target lost its table: %q", kept)
			}

			if _, err = admin.Exec(ctx, "ALTER DATABASE "+target+" OWNER TO "+role); err != nil {
				t.Fatal(err)
			}
			row["operation_id"] = strings.Repeat("b", 32)
			got = lawActivationSteps(t, activatorConfig, psqlPath, request, "choose", "make")
			mark()
			if want = []string{"new", "ok"}; !reflect.DeepEqual(got, want) {
				t.Fatalf("activator-owned target: got %q, want %q", got, want)
			}
			activatorTarget := activatorConfig.Copy()
			activatorTarget.Database = name
			if gone := frameworksPsql(t, psqlPath, activatorTarget, "", "SELECT to_regclass('public.hand_restored') IS NULL;\n"); gone != "t" {
				t.Fatalf("recreated target kept its table: %q", gone)
			}
		})
	}
}
