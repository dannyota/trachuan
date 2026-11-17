//go:build integration

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"danny.vn/trachuan/internal/testpg"
	"github.com/jackc/pgx/v5"
)

func TestFrameworksReleaseRow_InventedReleaseIgnoresWorkshop(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	ctx := t.Context()
	admin, base, err := testpg.OpenAdministrator(ctx)
	if err != nil {
		t.Fatal("open fixture administrator")
	}
	defer func() { _ = admin.Close(context.Background()) }()
	reserved, key, err := testpg.CreateCorpus(ctx, admin, t.Name())
	if err != nil {
		t.Fatal(err)
	}
	name := strings.Replace(reserved, "corpus_vn_", "frameworks_", 1)
	if !testpg.DropCorpusDatabase(t, admin, reserved, key) {
		t.Fatal("release fixture marker mismatch")
	}
	if _, err = admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	if _, err = admin.Exec(ctx, "COMMENT ON DATABASE "+pgx.Identifier{name}.Sanitize()+" IS '{\"run_key\":\""+key+"\"}'"); err != nil {
		t.Fatal(err)
	}
	defer testpg.DropCorpusDatabase(t, admin, name, key)
	base.Path = "/" + name
	conn, err := pgx.Connect(ctx, base.String())
	if err != nil {
		t.Fatal("connect invented release")
	}
	defer func() { _ = conn.Close(context.Background()) }()
	rel := strings.TrimPrefix(name, "frameworks_")
	if _, err = conn.Exec(ctx, `CREATE EXTENSION vector;
 CREATE TABLE release(singleton boolean PRIMARY KEY,kind text,rel text,schema_version int,control_count bigint,extra_recipe jsonb);`); err != nil {
		t.Fatal(err)
	}
	if _, err = conn.Exec(ctx, `INSERT INTO release VALUES(true,'frameworks',$1,1,7,'{"invented":true}')`, rel); err != nil {
		t.Fatal(err)
	}
	work := t.TempDir()
	t.Setenv("FRAMEWORKS_WORK_DIR", work)
	t.Setenv("CORPUS_BUILDER_DATABASE_URL", base.String())
	t.Setenv("CORPUS_DATABASE_URL", "postgres://invalid.invalid/workshop")
	var out, stderr bytes.Buffer
	if code := Run(ctx, []string{"frameworks", "release-row", rel}, &out, &stderr); code != 0 {
		t.Fatalf("release-row failed: %s", stderr.String())
	}
	data, err := os.ReadFile(filepath.Join(work, rel, "release", "release.json"))
	if err != nil {
		t.Fatal(err)
	}
	var row map[string]json.RawMessage
	if err = json.Unmarshal(data, &row); err != nil {
		t.Fatal(err)
	}
	if len(row) != 6 || row["singleton"] != nil || row["extra_recipe"] == nil {
		t.Fatal("release columns were not preserved")
	}
	var extensions map[string]string
	if err = json.Unmarshal(row["extensions"], &extensions); err != nil || len(extensions) != 1 || extensions["vector"] == "" {
		t.Fatal("wrong extension metadata")
	}
	cfg, err := frameworksReleaseRowConfig(base.String(), rel)
	if err != nil {
		t.Fatal(err)
	}
	read, err := pgx.ConnectConfig(ctx, cfg.ConnConfig)
	if err != nil {
		t.Fatal("connect read-only fixture")
	}
	defer func() { _ = read.Close(context.Background()) }()
	if _, err = read.Exec(ctx, `DELETE FROM release`); err == nil {
		t.Fatal("metadata reader wrote to database")
	}
}
