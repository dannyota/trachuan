//go:build integration

package releasecheck

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/server/internal/migrate"
	"danny.vn/trachuan/internal/testpg"
	"github.com/jackc/pgx/v5"
)

const journalCheckQuery = `WITH latest AS (
  SELECT DISTINCT ON (corpus_kind, operation_id)
    corpus_kind, operation_id, country, rel, mode, database_name,
    manifest_hash, dump_sha256, expected_active_db, step, manifest
  FROM public.release_journal
  WHERE operation_id IS NOT NULL
  ORDER BY corpus_kind, operation_id, id DESC
)
SELECT corpus_kind,
  pg_catalog.count(*) AS latest_rows,
  pg_catalog.count(*) FILTER (WHERE (operation_id ~ '^[0-9a-f]{32}$') IS NOT TRUE) AS bad_operation_id,
  pg_catalog.count(*) FILTER (WHERE (CASE corpus_kind WHEN 'law' THEN country ~ '^[a-z]{2}$' WHEN 'frameworks' THEN country ~ '^vn$' END) IS NOT TRUE) AS bad_country,
  pg_catalog.count(*) FILTER (WHERE (CASE corpus_kind WHEN 'law' THEN rel ~ '^[0-9]{8}(?:_[1-9][0-9]*)?$' WHEN 'frameworks' THEN rel ~ '^[0-9]{8}(?:_[0-9]{1,2})?$' END) IS NOT TRUE) AS bad_rel,
  pg_catalog.count(*) FILTER (WHERE (mode IN ('new', 'retained', 'prune')) IS NOT TRUE) AS bad_mode,
  pg_catalog.count(*) FILTER (WHERE (CASE corpus_kind WHEN 'law' THEN database_name ~ '^corpus_[a-z]{2}_[0-9]{8}(?:_[1-9][0-9]*)?$' WHEN 'frameworks' THEN database_name ~ '^frameworks_[0-9]{8}(?:_[0-9]{1,2})?$' END) IS NOT TRUE) AS bad_database_name,
  pg_catalog.count(*) FILTER (WHERE (manifest_hash ~ '^[0-9a-f]{64}$' OR (mode = 'prune' AND manifest_hash IS NULL)) IS NOT TRUE) AS bad_manifest_hash,
  pg_catalog.count(*) FILTER (WHERE (dump_sha256 ~ '^[0-9a-f]{64}$' OR (mode = 'prune' AND dump_sha256 IS NULL)) IS NOT TRUE) AS bad_dump_sha256,
  pg_catalog.count(*) FILTER (WHERE (expected_active_db IS NULL OR CASE corpus_kind WHEN 'law' THEN expected_active_db ~ '^corpus_[a-z]{2}_[0-9]{8}(?:_[1-9][0-9]*)?$' WHEN 'frameworks' THEN expected_active_db ~ '^frameworks_[0-9]{8}(?:_[0-9]{1,2})?$' END) IS NOT TRUE) AS bad_expected_active_db,
  pg_catalog.count(*) FILTER (WHERE (step IN ('started', 'creating', 'created', 'restoring', 'restored', 'validated', 'checked', 'activated', 'reverted', 'aborted', 'pruning', 'pruned')) IS NOT TRUE) AS bad_step,
  pg_catalog.count(*) FILTER (WHERE (pg_catalog.jsonb_typeof(manifest) = 'object' OR (mode = 'prune' AND (manifest IS NULL OR pg_catalog.jsonb_typeof(manifest) = 'null'))) IS NOT TRUE) AS bad_manifest
FROM latest
GROUP BY corpus_kind
ORDER BY corpus_kind;
`

const journalPointerQuery = `WITH names AS (
  SELECT 'law'::text AS corpus_kind, 'pointer'::text AS source, active_db AS name FROM public.corpus_release
  UNION ALL SELECT 'frameworks', 'pointer', active_db FROM public.frameworks_release
  UNION ALL SELECT corpus_kind, 'retired', db_name FROM public.retired_release
), kinds (corpus_kind, pattern) AS (
  VALUES ('law'::text, '^corpus_[a-z]{2}_[0-9]{8}(?:_[1-9][0-9]*)?$'::text), ('frameworks', '^frameworks_[0-9]{8}(?:_[0-9]{1,2})?$')
)
SELECT k.corpus_kind,
  pg_catalog.count(*) FILTER (WHERE n.source = 'pointer') AS pointer_rows,
  pg_catalog.count(*) FILTER (WHERE n.source = 'pointer' AND (n.name ~ k.pattern) IS NOT TRUE) AS bad_active_db,
  pg_catalog.count(*) FILTER (WHERE n.source = 'retired') AS retired_rows,
  pg_catalog.count(*) FILTER (WHERE n.source = 'retired' AND (n.name ~ k.pattern) IS NOT TRUE) AS bad_retired_db_name
FROM kinds k
LEFT JOIN names n ON n.corpus_kind = k.corpus_kind
GROUP BY k.corpus_kind
ORDER BY k.corpus_kind;
`

const journalHarness = `import hashlib
import json
from pathlib import Path
import sys
import tempfile
import uuid

root, phase = Path(sys.argv[1]), sys.argv[2]
sys.path.insert(0, str(root / 'deploy/host'))

from updater.deploy_corpus_activate import Activation
from updater.deploy_corpus_database import Database
from updater.deploy_corpus_kind import FRAMEWORKS, LAW
from updater.deploy_corpus_sql import JournalRefused, STEPS
from updater.deploy_host import Host
from updater.deploy_io import command
from updater.deploy_store import Paths

target = json.load(sys.stdin)
fixtures = root / 'deploy/host/updater/fixtures'
KINDS = (('law', LAW, 'corpus-manifest.json', 'corpus_vn_20990101_1', 'corpus_vn_20990101_1',
          'corpus_vn_20990103_1', 'c' * 32),
         ('frameworks', FRAMEWORKS, 'frameworks-manifest.json', None, 'frameworks_20990101',
          'frameworks_20990103', 'd' * 32))


class Held:
    def poll(self):
        return None


def database(kind, work, log):
    def runner(args, **options):
        if args[:6] != ['podman', 'exec', '-i', 'trachuan-postgres', 'sh', '-c'] or args[7:9] != ['sh', 'psql']:
            raise RuntimeError('unexpected client command')
        client = list(args[8:])
        if client[client.index('-d') + 1] != 'auth':
            raise RuntimeError('unexpected client database')
        client[0] = target['psql']
        for flag, value in (('-h', target['host']), ('-U', target['user']), ('-d', target['database'])):
            client[client.index(flag) + 1] = value
        client[1:1] = ['-p', str(target['port'])]
        log.append(options['input'].decode())
        return command(client, **options)
    db = Database(Host(Paths(Path(work)), runner, user_prefix=[]), {'extensions': ['vector', 'ltree']}, kind)
    db.holder = Held()
    return db


def latest(db, row, step):
    found = {item['operation_id']: item for item in db.operations()}
    if found[row['operation_id']]['step'] != step:
        raise RuntimeError('journal step was not read back')


def roundtrip(kind, db, digest, manifest, expected, pruned):
    engine, seen = Activation(db, kind), []
    def advance(row, step):
        row = db.append(row, step)
        latest(db, row, step)
        seen.append(step)
        return row
    row = engine.begin(digest, manifest, 'new', expected)
    latest(db, row, 'started')
    seen.append('started')
    for step in ('creating', 'created', 'restoring', 'restored', 'validated', 'checked'):
        row = advance(row, step)
    row = db.activate(row)
    latest(db, row, 'activated')
    seen.append('activated')
    advance(engine.begin(digest, manifest, 'retained', expected), 'aborted')
    advance(engine.begin(digest, manifest, 'new', expected), 'reverted')
    country, rel = kind.parse(pruned)
    prune = dict(operation_id=uuid.uuid4().hex, country=country, rel=rel, mode='prune', database_name=pruned,
                 manifest_hash=None, dump_sha256=None, expected_active_db=None, manifest=None)
    advance(advance(prune, 'pruning'), 'pruned')
    if set(seen) != set(STEPS):
        raise RuntimeError('journal steps not covered')
    operations = [dict(mode=item['mode'], step=item['step'], expected_null=item['expected_active_db'] is None,
                       manifest_matches=item['manifest'] == (None if item['mode'] == 'prune' else manifest))
                  for item in db.operations()]
    return dict(steps=sorted(seen), operations=sorted(operations, key=lambda item: item['step']),
                active=db.current('vn'))


def tampered(kind, db, log, digest, manifest, name, operation):
    active = db.current('vn')
    row = dict(operation_id=operation, country=kind.country({'country': 'vn'}), rel=kind.parse(name)[1], mode='new',
               database_name=name, manifest_hash=digest, dump_sha256=manifest['dump']['sha256'],
               expected_active_db=active, manifest=manifest, step='created')
    refused = False
    log.clear()
    try:
        Activation(db, kind).resume(row, None, 'image')
    except JournalRefused:
        refused = True
    inserts = [index for index, text in enumerate(log) if 'INSERT INTO release_journal' in text]
    return dict(refused=refused, inserts=len(inserts), after_insert=len(log) - 1 - inserts[-1], operation=operation)


def literals(kind, db, digest, manifest, expected, operation):
    body = dict(manifest, probe=target['probe'])
    row = dict(operation_id=operation, country=kind.country({'country': 'vn'}), rel=manifest['rel'], mode='new',
               database_name=manifest['database'], manifest_hash=digest, dump_sha256=manifest['dump']['sha256'],
               expected_active_db=expected, manifest=body)
    row = db.append(db.append(row, 'started'), 'checked')
    db.activate(row)
    found = {item['operation_id']: item for item in db.operations()}[operation]
    if found['manifest'] != body or found['step'] != 'activated':
        raise RuntimeError('journal manifest was misread')
    return dict(operation=operation)


result = {}
with tempfile.TemporaryDirectory() as work:
    for name, kind, fixture, expected, pruned, successor, operation in KINDS:
        raw = (fixtures / fixture).read_bytes()
        digest, manifest, log = hashlib.sha256(raw).hexdigest(), json.loads(raw), []
        db = database(kind, work, log)
        if phase == 'roundtrip':
            result[name] = roundtrip(kind, db, digest, manifest, expected, pruned)
        elif phase == 'literals':
            result[name] = literals(kind, db, digest, manifest, expected, operation)
        else:
            result[name] = tampered(kind, db, log, digest, manifest, successor, operation)
print(json.dumps(result))
`

type journalOperation struct {
	Mode            string `json:"mode"`
	Step            string `json:"step"`
	ExpectedNull    bool   `json:"expected_null"`
	ManifestMatches bool   `json:"manifest_matches"`
}

type journalResult struct {
	Steps       []string           `json:"steps"`
	Operations  []journalOperation `json:"operations"`
	Active      string             `json:"active"`
	Refused     bool               `json:"refused"`
	Inserts     int                `json:"inserts"`
	AfterInsert int                `json:"after_insert"`
	Operation   string             `json:"operation"`
}

func journalPsql(t *testing.T) string {
	t.Helper()
	path := pgClientPath("psql", "")
	if dump := os.Getenv("CORPUS_PG_DUMP"); dump != "" {
		path = filepath.Join(filepath.Dir(dump), "psql")
	}
	output, err := exec.CommandContext(t.Context(), path, "--version").Output()
	if err != nil {
		t.Fatalf("get psql version: %v", err)
	}
	major, err := pgClientMajorVersion("psql", string(output))
	if err != nil {
		t.Fatal(err)
	}
	if major < 18 {
		if os.Getenv("CI") == "true" {
			t.Fatalf("needs psql 18 or newer (found %d)", major)
		}
		t.Skipf("needs psql 18 or newer (found %d); on macOS put /opt/homebrew/opt/postgresql@18/bin first on PATH", major)
	}
	return path
}

type journalFixture struct {
	auth    *pgx.Conn
	authURL string
	config  *pgx.ConnConfig
	psql    string
}

func newJournalFixture(t *testing.T) journalFixture {
	t.Helper()
	ctx := t.Context()
	psql := journalPsql(t)
	builder := testpg.NewBuilder(t)
	authURL := testpg.NewDatabase(t)
	if err := migrate.Up(ctx, authURL); err != nil {
		t.Fatal(err)
	}
	auth, err := pgx.Connect(ctx, authURL)
	if err != nil {
		t.Fatal("connect fixture auth")
	}
	t.Cleanup(func() { _ = auth.Close(context.Background()) })
	role := pgx.Identifier{builder.Role}.Sanitize()
	if _, err = auth.Exec(ctx, "GRANT USAGE ON SCHEMA public TO "+role+"; GRANT SELECT,INSERT,UPDATE,DELETE ON corpus_release,release_journal,retired_release TO "+role+"; GRANT USAGE,SELECT ON SEQUENCE release_journal_id_seq TO "+role+"; GRANT SELECT,INSERT,UPDATE ON frameworks_release TO "+role); err != nil {
		t.Fatal(err)
	}
	if _, err = auth.Exec(ctx, `INSERT INTO corpus_release VALUES ('vn','corpus_vn_20990101_1',now(),'manual')`); err != nil {
		t.Fatal(err)
	}
	config, err := pgx.ParseConfig(builder.URL)
	if err != nil {
		t.Fatal("parse synthetic activator configuration")
	}
	config.Database = auth.Config().Database
	return journalFixture{auth: auth, authURL: authURL, config: config, psql: psql}
}

func runJournalHarness(t *testing.T, fixture journalFixture, phase, probe string) map[string]journalResult {
	t.Helper()
	config := fixture.config
	root, err := filepath.Abs("../../../..")
	if err != nil {
		t.Fatal(err)
	}
	input, err := json.Marshal(map[string]any{"psql": fixture.psql, "host": config.Host, "port": config.Port,
		"user": config.User, "database": config.Database, "probe": probe})
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(t.Context(), "python3", "-I", "-B", "-c", journalHarness, root, phase)
	cmd.Env = append(os.Environ(), "PGPASSWORD="+config.Password, "PGOPTIONS=", "PGCONNECT_TIMEOUT=5")
	cmd.Stdin = bytes.NewReader(input)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("journal harness %s: %v: %s", phase, err, stderr.String())
	}
	var result map[string]journalResult
	if err = json.Unmarshal(output, &result); err != nil {
		t.Fatalf("decode journal harness output: %v", err)
	}
	return result
}

func journalCounts(t *testing.T, conn *pgx.Conn, query string) map[string]map[string]int64 {
	t.Helper()
	rows, err := conn.Query(t.Context(), query)
	if err != nil {
		t.Fatalf("run journal check query: %v", err)
	}
	records, err := pgx.CollectRows(rows, pgx.RowToMap)
	if err != nil {
		t.Fatalf("read journal check query: %v", err)
	}
	counts := map[string]map[string]int64{}
	for _, record := range records {
		kind, ok := record["corpus_kind"].(string)
		if !ok {
			t.Fatal("journal check query returned no kind")
		}
		counts[kind] = map[string]int64{}
		for column, value := range record {
			if column == "corpus_kind" {
				continue
			}
			count, ok := value.(int64)
			if !ok {
				t.Fatalf("journal check column %s is not a count", column)
			}
			counts[kind][column] = count
		}
	}
	return counts
}

func assertJournalCounts(t *testing.T, counts map[string]map[string]int64, want map[string]map[string]int64) {
	t.Helper()
	for _, kind := range []string{"law", "frameworks"} {
		columns, ok := counts[kind]
		if !ok {
			t.Fatalf("journal check returned no %s row", kind)
		}
		if columns["latest_rows"] <= 0 {
			t.Errorf("%s latest_rows = %d", kind, columns["latest_rows"])
		}
		for column, count := range columns {
			if strings.HasPrefix(column, "bad_") && count != want[kind][column] {
				t.Errorf("%s %s = %d; want %d", kind, column, count, want[kind][column])
			}
		}
	}
}

func assertPointerCounts(t *testing.T, conn *pgx.Conn, want map[string]map[string]int64) {
	t.Helper()
	counts := journalCounts(t, conn, journalPointerQuery)
	for kind, columns := range want {
		for column, count := range columns {
			if counts[kind][column] != count {
				t.Errorf("%s %s = %d; want %d", kind, column, counts[kind][column], count)
			}
		}
	}
}

func TestActivationJournal_RealPsqlRoundTripAndTamperedInsert(t *testing.T) {
	ctx := t.Context()
	fixture := newJournalFixture(t)
	auth := fixture.auth
	var err error

	steps := []string{"aborted", "activated", "checked", "created", "creating", "pruned", "pruning", "restored",
		"restoring", "reverted", "started", "validated"}
	active := map[string]string{"law": "corpus_vn_20990102_1", "frameworks": "frameworks_20990102"}
	modes := map[string]string{"aborted": "retained", "activated": "new", "pruned": "prune", "reverted": "new"}
	result := runJournalHarness(t, fixture, "roundtrip", "")
	for kind, want := range active {
		got := result[kind]
		if !slices.Equal(got.Steps, steps) {
			t.Errorf("%s steps = %v", kind, got.Steps)
		}
		if got.Active != want {
			t.Errorf("%s active = %q; want %q", kind, got.Active, want)
		}
		if len(got.Operations) != len(modes) {
			t.Fatalf("%s operations = %+v", kind, got.Operations)
		}
		for _, operation := range got.Operations {
			wantNull := operation.Step == "pruned" || kind == "frameworks"
			if operation.Mode != modes[operation.Step] || operation.ExpectedNull != wantNull || !operation.ManifestMatches {
				t.Errorf("%s operation = %+v", kind, operation)
			}
		}
	}
	assertJournalCounts(t, journalCounts(t, auth, journalCheckQuery), nil)
	assertPointerCounts(t, auth, map[string]map[string]int64{
		"law":        {"pointer_rows": 1, "bad_active_db": 0, "retired_rows": 1, "bad_retired_db_name": 0},
		"frameworks": {"pointer_rows": 1, "bad_active_db": 0, "retired_rows": 0, "bad_retired_db_name": 0},
	})

	if _, err = auth.Exec(ctx, `CREATE FUNCTION change_journal_country() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN NEW.country := 'zz'; RETURN NEW; END; $$;
		CREATE TRIGGER change_journal_country BEFORE INSERT ON release_journal FOR EACH ROW EXECUTE FUNCTION change_journal_country();`); err != nil {
		t.Fatal(err)
	}
	tampered := runJournalHarness(t, fixture, "tampered", "")
	for _, kind := range []string{"law", "frameworks"} {
		got := tampered[kind]
		if !got.Refused || got.Inserts != 1 || got.AfterInsert != 0 {
			t.Errorf("%s tampered insert = %+v", kind, got)
		}
		var count int
		var step, country string
		if err = auth.QueryRow(ctx, `SELECT count(*), min(step), min(country) FROM release_journal WHERE operation_id=$1`, got.Operation).Scan(&count, &step, &country); err != nil {
			t.Fatal(err)
		}
		if count != 1 || step != "restoring" || country != "zz" {
			t.Errorf("%s stored rows = %d %q %q", kind, count, step, country)
		}
	}
	want := map[string]map[string]int64{"frameworks": {"bad_country": 1}}
	assertJournalCounts(t, journalCounts(t, auth, journalCheckQuery), want)

	if _, err = auth.Exec(ctx, `INSERT INTO release_journal (operation_id,country,rel,mode,database_name,step,state,message)
		VALUES (repeat('e',32),'vn','not-a-release','prune','corpus_vn_20990104','started','open','fixture')`); err != nil {
		t.Fatal(err)
	}
	want["law"] = map[string]int64{"bad_rel": 1}
	assertJournalCounts(t, journalCounts(t, auth, journalCheckQuery), want)

	if _, err = auth.Exec(ctx, `ALTER TABLE frameworks_release DROP CONSTRAINT frameworks_release_active_db;
		ALTER TABLE retired_release DROP CONSTRAINT retired_release_kind;
		INSERT INTO corpus_release VALUES ('zz','host=127.0.0.1 dbname=x',now(),'fixture');
		UPDATE frameworks_release SET active_db='postgresql://127.0.0.1/x';
		INSERT INTO retired_release (country,db_name,retired_at,retain_until,corpus_kind) VALUES
			('vn','frameworks_20990101',now(),now(),'law'),('vn','corpus_vn_20990101',now(),now(),'frameworks');`); err != nil {
		t.Fatal(err)
	}
	assertPointerCounts(t, auth, map[string]map[string]int64{
		"law":        {"pointer_rows": 2, "bad_active_db": 1, "retired_rows": 2, "bad_retired_db_name": 1},
		"frameworks": {"pointer_rows": 1, "bad_active_db": 1, "retired_rows": 1, "bad_retired_db_name": 1},
	})
}

func TestActivationJournal_LiteralsSurviveStandardConformingStringsOff(t *testing.T) {
	ctx := t.Context()
	fixture := newJournalFixture(t)
	database := pgx.Identifier{fixture.config.Database}.Sanitize()
	if _, err := fixture.auth.Exec(ctx, "ALTER DATABASE "+database+" SET standard_conforming_strings = off"); err != nil {
		t.Fatal(err)
	}
	session, err := pgx.Connect(ctx, fixture.authURL)
	if err != nil {
		t.Fatal("connect after changing the database setting")
	}
	defer func() { _ = session.Close(context.Background()) }()
	var setting string
	if err = session.QueryRow(ctx, `SHOW standard_conforming_strings`).Scan(&setting); err != nil {
		t.Fatal(err)
	}
	if setting != "off" {
		t.Fatalf("standard_conforming_strings = %q", setting)
	}
	marker := filepath.Join(t.TempDir(), "executed")
	probe := `back\slash ' quote \' \! touch ` + marker + "\n"
	result := runJournalHarness(t, fixture, "literals", probe)
	for _, kind := range []string{"law", "frameworks"} {
		var count int
		var low, high string
		if err = fixture.auth.QueryRow(ctx, `SELECT count(*), min(manifest->>'probe'), max(manifest->>'probe') FROM release_journal WHERE operation_id=$1`, result[kind].Operation).Scan(&count, &low, &high); err != nil {
			t.Fatal(err)
		}
		if count != 3 || low != probe || high != probe {
			t.Errorf("%s stored %d rows, probe %q to %q", kind, count, low, high)
		}
	}
	if _, err = os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("psql ran a command from a manifest literal: %v", err)
	}
}

func TestActivationJournal_RunbookKeepsTheCheckQueries(t *testing.T) {
	runbook, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "docs", "runbooks", "release-journal-check.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{journalCheckQuery, journalPointerQuery} {
		if !strings.Contains(string(runbook), strings.TrimSpace(query)) {
			t.Fatalf("runbook does not hold the query starting %q", strings.TrimSpace(query)[:30])
		}
	}
}
