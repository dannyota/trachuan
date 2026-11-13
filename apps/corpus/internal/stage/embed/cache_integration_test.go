//go:build integration

package embed

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/stage/chunk"
	"danny.vn/trachuan/apps/corpus/internal/testdb"
	"danny.vn/trachuan/apps/corpus/migrations"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

func TestEmbedRow_MigrationReverts(t *testing.T) {
	ctx := context.Background()
	cfg, err := pgx.ParseConfig(testdb.NewEmpty(t))
	if err != nil {
		t.Fatal(err)
	}
	db := stdlib.OpenDB(*cfg)
	defer func() { _ = db.Close() }()
	p, err := goose.NewProvider(goose.DialectPostgres, db, migrations.FS)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = p.UpTo(ctx, 10); err != nil {
		t.Fatal(err)
	}
	if _, err = p.Down(ctx); err != nil {
		t.Fatal(err)
	}
	var present bool
	if err = db.QueryRowContext(ctx, "SELECT to_regclass('embed_row') IS NOT NULL").Scan(&present); err != nil || present {
		t.Fatalf("present=%v err=%v", present, err)
	}
	if _, err = p.Up(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestEmbedRow_WriterInsertsAndIgnoresConflicts(t *testing.T) {
	ctx := context.Background()
	db, err := pgxpool.New(ctx, testdb.New(t))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	artifacts := artifact.Store{Root: t.TempDir()}
	f := &fake{}
	s := New(f.client(recipe()), recipe())
	input := strings.Repeat("b", 64)
	idx, vectors, err := s.build(ctx, chunk.File{DocumentKey: "doc", Chunks: []chunk.Chunk{{Content: "a"}}}, chunk.Coverage{}, input)
	if err != nil {
		t.Fatal(err)
	}
	art, err := artifacts.Publish(ctx, "embed", input, s.Recipe(stage.Snapshot{}), func(dir string) error {
		b, e := json.Marshal(idx)
		if e != nil {
			return e
		}
		if e = os.WriteFile(filepath.Join(dir, "index.json"), b, 0600); e != nil {
			return e
		}
		return os.WriteFile(filepath.Join(dir, "vectors.f32"), vectors, 0600)
	})
	if err != nil {
		t.Fatal(err)
	}
	var id int64
	if err = db.QueryRow(ctx, "INSERT INTO artifact(stage,item_key,input_hash,recipe_hash,path,sha256) VALUES('embed','doc',$1,$2,$3,$4) RETURNING id", input, s.Recipe(stage.Snapshot{}), art.Path, art.SHA256).Scan(&id); err != nil {
		t.Fatal(err)
	}
	var otherID int64
	if err = db.QueryRow(ctx, "INSERT INTO artifact(stage,item_key,input_hash,recipe_hash,path,sha256) VALUES('embed','other',$1,$2,$3,$4) RETURNING id", strings.Repeat("c", 64), s.Recipe(stage.Snapshot{}), art.Path, art.SHA256).Scan(&otherID); err != nil {
		t.Fatal(err)
	}
	writer, err := s.Rows(ctx, stage.Env{DB: db, Artifacts: artifacts}, art)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err = writer(ctx, tx, id); err != nil {
		t.Fatal(err)
	}
	if err = writer(ctx, tx, otherID); err != nil {
		t.Fatal(err)
	}
	var count int
	var gotID int64
	if err = tx.QueryRow(ctx, "SELECT count(*), min(artifact_id) FROM embed_row WHERE recipe_hash=$1", s.Recipe(stage.Snapshot{})).Scan(&count, &gotID); err != nil || count != 1 || gotID != id {
		t.Fatalf("count=%d id=%d err=%v", count, gotID, err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}
