//go:build integration

package fetch

import (
	"context"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/testdb"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestFetch_WritesBundleAndRows(t *testing.T) {
	t.Parallel()
	f := &fixtureServer{files: []map[string]any{entry("a", "a")}, bodies: map[string]string{"a": "content"}}
	s, env := setup(t, f)
	out := process(t, s, env)
	rows, err := s.Rows(context.Background(), env, *out.Artifact)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	dbURL := testdb.New(t)
	db, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var id int64
	err = tx.QueryRow(ctx, `INSERT INTO artifact (stage,item_key,input_hash,recipe_hash,path,sha256) VALUES ('fetch','vn:vbpl:66518',$1,$2,$3,$1) RETURNING id`, out.Artifact.SHA256, s.Recipe(stage.Snapshot{}), out.Artifact.Path).Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	if err = rows(ctx, tx, id); err != nil {
		t.Fatal(err)
	}
	var key, number, status, name, hash, path, source, role string
	var sourceURL *string
	var size int64
	err = tx.QueryRow(ctx, `SELECT key,number,status_code,source_url FROM document WHERE artifact_id=$1`, id).Scan(&key, &number, &status, &sourceURL)
	if err != nil {
		t.Fatal(err)
	}
	if sourceURL == nil || *sourceURL != "https://vbpl.vn/van-ban/chi-tiet/66518" {
		t.Fatalf("source_url = %v", sourceURL)
	}
	if key != "vn:vbpl:66518" || number != "04/VBHN-NHNN" || status != "" {
		t.Fatalf("document: %q %q %q", key, number, status)
	}
	err = tx.QueryRow(ctx, `SELECT name,sha256,size,path,source,role FROM fetched_file WHERE artifact_id=$1`, id).Scan(&name, &hash, &size, &path, &source, &role)
	if err != nil {
		t.Fatal(err)
	}
	if name != "a" || size != 7 || path != "files/"+hash || source != "vbpl" || role != "unknown" {
		t.Fatalf("file: %q %q %d %q", name, hash, size, path)
	}
}
