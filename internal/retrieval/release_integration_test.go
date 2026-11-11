//go:build integration

package retrieval

import (
	"context"
	"testing"

	"danny.vn/trachuan/internal/corpusdb"
	"danny.vn/trachuan/internal/embedding"
)

func TestReadRelease_ReturnsReleaseQueryRecipe(t *testing.T) {
	db, _, _ := fixture(t)
	release, err := ReadRelease(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	if release.Country != "vn" || release.Rel != "test" || release.SchemaVersion != corpusdb.SchemaVersion || release.DenseRecipe.QueryHash != embedding.QueryRecipe1().Hash() {
		t.Fatal("release identity or query recipe differs from the stored row")
	}
}

func TestReadRelease_RejectsUnsupportedSchemaAndMalformedMetadata(t *testing.T) {
	for _, tt := range []struct{ name, sql string }{
		{"schema 3", `ALTER TABLE release DROP CONSTRAINT release_schema_version_check; UPDATE release SET schema_version=3`},
		{"schema 6", `ALTER TABLE release DROP CONSTRAINT release_schema_version_check; UPDATE release SET schema_version=6`},
		{"metadata", `UPDATE release SET dense_recipe='{"version":"bad"}'`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			db, _, embedder := fixture(t)
			if _, err := db.Exec(context.Background(), tt.sql); err != nil {
				t.Fatal(err)
			}
			if _, err := ReadRelease(context.Background(), db); err == nil {
				t.Fatal("read accepted invalid metadata")
			}
			if _, err := Open(context.Background(), db, embedder, fixtureOptions()); err == nil {
				t.Fatal("open accepted invalid metadata")
			}
		})
	}
}

func TestOpen_Version4HasNoNodes(t *testing.T) {
	db, _, embedder := fixture(t)
	ctx := context.Background()
	_, err := db.Exec(ctx, `ALTER TABLE release DROP CONSTRAINT release_schema_version_check; ALTER TABLE release ADD CONSTRAINT release_schema_version_check CHECK (schema_version=4) NOT VALID; UPDATE release SET schema_version=4; ALTER TABLE taxonomy_node DROP COLUMN ordinal, DROP COLUMN provision_count, DROP COLUMN document_count; INSERT INTO taxonomy_node(id,country,level,code,name,description,origin,status,path) VALUES(1,'vn',1,'one','','','custom','active','n1')`)
	if err != nil {
		t.Fatal(err)
	}
	engine, err := Open(ctx, db, embedder, fixtureOptions())
	if err != nil {
		t.Fatal(err)
	}
	status, err := engine.Status(ctx)
	if err != nil || status.Nodes != 0 {
		t.Fatalf("status nodes = %d, error = %v", status.Nodes, err)
	}
}
