//go:build integration

package releasecheck

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/server/internal/migrate"
	"danny.vn/trachuan/internal/testpg"
	"github.com/jackc/pgx/v5"
)

func pythonSQL(t *testing.T, function string, value any) string {
	t.Helper()
	root, err := filepath.Abs("../../../..")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	script := `import json,sys
sys.path.insert(0,sys.argv[1])
from updater import deploy_corpus_sql as sql
value=json.load(sys.stdin)
action=sys.argv[2]
print(sql.activate(value) if action=='activate' else sql.protected(value) if action=='protected' else sql.journal(value[0],value[1]))
`
	cmd := exec.CommandContext(t.Context(), "python3", "-c", script, filepath.Join(root, "deploy/host"), function)
	cmd.Stdin = bytes.NewReader(raw)
	output, err := cmd.Output()
	if err != nil {
		t.Fatal("generate production activation SQL")
	}
	return string(output)
}

func pgCommand(t *testing.T, config *pgx.ConnConfig, program string, input []byte, args ...string) []byte {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), program, append([]string{"-h", config.Host, "-p", strconv.Itoa(int(config.Port)), "-U", config.User}, args...)...)
	cmd.Env = append(os.Environ(), "PGPASSWORD="+config.Password, "PGOPTIONS=", "PGCONNECT_TIMEOUT=5")
	cmd.Stdin = bytes.NewReader(input)
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("fixture %s failed", program)
	}
	return output
}

func TestActivation_RestoreCASRetainedAndPrune(t *testing.T) {
	ctx := context.Background()
	admin, base, err := testpg.OpenAdministrator(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Close(ctx) })
	builder := testpg.NewBuilder(t)
	var names []string
	for _, suffix := range []string{"manual", "restored", "successor"} {
		name, key, createErr := testpg.CreateCorpus(ctx, admin, t.Name()+suffix)
		if createErr != nil {
			t.Fatal(createErr)
		}
		t.Cleanup(func() { testpg.DropCorpusDatabase(t, admin, name, key) })
		if _, err = admin.Exec(ctx, "ALTER DATABASE "+pgx.Identifier{name}.Sanitize()+" OWNER TO "+pgx.Identifier{builder.Role}.Sanitize()); err != nil {
			t.Fatal(err)
		}
		names = append(names, name)
	}
	authURL := testpg.NewDatabase(t)
	if err = migrate.Up(ctx, authURL); err != nil {
		t.Fatal(err)
	}
	auth, err := pgx.Connect(ctx, authURL)
	if err != nil {
		t.Fatal("connect fixture auth")
	}
	t.Cleanup(func() { _ = auth.Close(ctx) })
	if _, err = auth.Exec(ctx, "GRANT USAGE ON SCHEMA public TO "+pgx.Identifier{builder.Role}.Sanitize()+"; GRANT SELECT,INSERT,UPDATE,DELETE ON corpus_release,release_journal,retired_release TO "+pgx.Identifier{builder.Role}.Sanitize()+"; GRANT USAGE,SELECT ON SEQUENCE release_journal_id_seq TO "+pgx.Identifier{builder.Role}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	if _, err = auth.Exec(ctx, `INSERT INTO corpus_release VALUES ('vn',$1,now(),'manual')`, names[0]); err != nil {
		t.Fatal(err)
	}
	activatorConfig, err := pgx.ParseConfig(builder.URL)
	if err != nil {
		t.Fatal("parse synthetic activator configuration")
	}
	activatorConfig.Database = auth.Config().Database
	activator, err := pgx.ConnectConfig(ctx, activatorConfig)
	if err != nil {
		t.Fatal("connect synthetic activator")
	}
	t.Cleanup(func() { _ = activator.Close(ctx) })
	fixture := testpg.NewDatabase(t, "vector", "ltree")
	source, err := pgx.Connect(ctx, fixture)
	if err != nil {
		t.Fatal("connect dump fixture")
	}
	defer func() { _ = source.Close(ctx) }()
	if _, err = source.Exec(ctx, `CREATE TABLE release (rel text, embedding vector(3), path ltree); INSERT INTO release VALUES ('synthetic','[1,0,0]','law.article');`); err != nil {
		t.Fatal(err)
	}
	dump := pgCommand(t, source.Config(), "pg_dump", nil, "-Fc", "--no-owner", "--no-privileges", source.Config().Database)
	listing := pgCommand(t, source.Config(), "pg_restore", dump, "--list")
	root, err := filepath.Abs("../../../..")
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(ctx, "python3", "-c", `import sys;sys.path.insert(0,sys.argv[1]);from updater.deploy_corpus_verify import toc;print(toc(sys.stdin.read(),['vector','ltree']),end='')`, filepath.Join(root, "deploy/host"))
	cmd.Stdin = bytes.NewReader(listing)
	selected, err := cmd.Output()
	if err != nil {
		t.Fatal("filter dump TOC")
	}
	listPath := filepath.Join(t.TempDir(), "restore.list")
	if err = os.WriteFile(listPath, selected, 0o600); err != nil {
		t.Fatal(err)
	}
	targetURL := *base
	targetURL.Path = "/" + names[1]
	target, err := pgx.Connect(ctx, targetURL.String())
	if err != nil {
		t.Fatal("connect restore target administrator")
	}
	if _, err = target.Exec(ctx, `CREATE EXTENSION vector; CREATE EXTENSION ltree;`); err != nil {
		t.Fatal(err)
	}
	if err = target.Close(ctx); err != nil {
		t.Fatal(err)
	}
	restoreConfig := activatorConfig.Copy()
	restoreConfig.Database = names[1]
	pgCommand(t, restoreConfig, "pg_restore", dump, "--dbname", names[1], "--no-owner", "--no-privileges", "--exit-on-error", "--single-transaction", "-L", listPath)
	restored, err := pgx.ConnectConfig(ctx, restoreConfig)
	if err != nil {
		t.Fatal("connect restored corpus as synthetic activator")
	}
	var path string
	readErr := restored.QueryRow(ctx, `SELECT path::text FROM release WHERE embedding <=> '[1,0,0]'::vector = 0`).Scan(&path)
	closeErr := restored.Close(ctx)
	if readErr != nil || closeErr != nil || path != "law.article" {
		t.Fatalf("restored vector and ltree data: read=%v close=%v path=%q", readErr, closeErr, path)
	}
	digest := sha256.Sum256(dump)
	row := map[string]any{"operation_id": strings.Repeat("1", 32), "country": "vn", "rel": strings.TrimPrefix(names[1], "corpus_vn_"), "mode": "new", "database_name": names[1], "manifest_hash": strings.Repeat("a", 64), "dump_sha256": hex.EncodeToString(digest[:]), "expected_active_db": names[0], "manifest": map[string]any{"fixture": true}}
	apply := func(value map[string]any) {
		t.Helper()
		if _, applyErr := activator.Exec(ctx, pythonSQL(t, "activate", value)); applyErr != nil {
			t.Fatal(applyErr)
		}
	}
	apply(row)
	var oldDigest *string
	if err = auth.QueryRow(ctx, `SELECT dump_sha256 FROM retired_release WHERE db_name=$1`, names[0]).Scan(&oldDigest); err != nil || oldDigest != nil {
		t.Fatalf("manual release digest: %v", err)
	}
	if _, err = activator.Exec(ctx, pythonSQL(t, "activate", row)); err == nil {
		t.Fatal("stale compare-and-swap succeeded")
	}
	if _, err = activator.Exec(ctx, "ROLLBACK"); err != nil {
		t.Fatal(err)
	}
	row["operation_id"], row["rel"], row["database_name"], row["expected_active_db"] = strings.Repeat("2", 32), strings.TrimPrefix(names[2], "corpus_vn_"), names[2], names[1]
	apply(row)
	row["operation_id"], row["rel"], row["database_name"], row["expected_active_db"], row["mode"] = strings.Repeat("3", 32), strings.TrimPrefix(names[1], "corpus_vn_"), names[1], names[2], "retained"
	apply(row)
	var count int
	if err = auth.QueryRow(ctx, `SELECT count(*) FROM retired_release WHERE db_name=$1`, names[1]).Scan(&count); err != nil || count != 0 {
		t.Fatalf("retained target not removed: %v", err)
	}
	var protected bool
	if err = activator.QueryRow(ctx, pythonSQL(t, "protected", names[2])).Scan(&protected); err != nil || !protected {
		t.Fatalf("retained database not protected: %v", err)
	}
	if _, err = auth.Exec(ctx, `UPDATE retired_release SET retain_until=now()-interval '1 second' WHERE db_name=$1`, names[2]); err != nil {
		t.Fatal(err)
	}
	if err = activator.QueryRow(ctx, pythonSQL(t, "protected", names[2])).Scan(&protected); err != nil || protected {
		t.Fatalf("expired database protected: %v", err)
	}
	prune := map[string]any{"operation_id": strings.Repeat("4", 32), "country": "vn", "rel": strings.TrimPrefix(names[2], "corpus_vn_"), "mode": "prune", "database_name": names[2], "manifest_hash": nil, "dump_sha256": nil, "expected_active_db": nil, "manifest": nil}
	if _, err = activator.Exec(ctx, pythonSQL(t, "journal", []any{prune, "pruning"})); err != nil {
		t.Fatal(err)
	}
	testpg.DropCorpusDatabase(t, admin, names[2], "vn/"+strings.TrimPrefix(names[2], "corpus_vn_"))
	if _, err = activator.Exec(ctx, `DELETE FROM retired_release WHERE db_name=$1`, names[2]); err != nil {
		t.Fatal(err)
	}
	if _, err = activator.Exec(ctx, pythonSQL(t, "journal", []any{prune, "pruned"})); err != nil {
		t.Fatal(err)
	}
}
