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
sys.path.insert(0,sys.argv[1])
from updater.deploy_corpus_activate import Activation
from updater.deploy_corpus_database import Database
from updater.deploy_corpus_kind import FRAMEWORKS, LAW
from updater.deploy_host import Host
from updater.deploy_io import command
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
db=Database(Host(Paths(),runner,user_prefix=[]),{'extensions':['vector','ltree']},kind)
db.holder=Held()
engine=Activation(db,kind)
actions={'choose':lambda:engine.choose(c['manifest'])[0],'make':lambda:db.make(c['row']) or 'ok','abort':lambda:engine.abort(c['row']) or 'ok'}
results=[]
for name in c['actions']:
    try:
        results.append(actions[name]())
    except ValueError as error:
        results.append('refused: '+str(error))
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

			authURL := testpg.NewDatabase(t)
			if err = migrate.Up(ctx, authURL); err != nil {
				t.Fatal(err)
			}
			auth, err := pgx.Connect(ctx, authURL)
			if err != nil {
				t.Fatal("connect fixture auth")
			}
			t.Cleanup(func() { _ = auth.Close(context.Background()) })
			role := pgx.Identifier{activator.Role}.Sanitize()
			if _, err = auth.Exec(ctx, "GRANT USAGE ON SCHEMA public TO "+role+"; GRANT SELECT,INSERT,UPDATE,DELETE ON corpus_release,release_journal,retired_release TO "+role+"; GRANT USAGE,SELECT ON SEQUENCE release_journal_id_seq TO "+role+"; GRANT SELECT,INSERT,UPDATE ON frameworks_release TO "+role); err != nil {
				t.Fatal(err)
			}
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
			request := map[string]any{"kind": tc.kind, "auth": auth.Config().Database, "manifest": manifest, "row": row}

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
