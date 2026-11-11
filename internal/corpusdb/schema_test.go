package corpusdb

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

type recordingDB struct{ calls int }

func (db *recordingDB) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	db.calls++
	return pgconn.CommandTag{}, nil
}

func TestApply_RejectsUnsupportedDimensions(t *testing.T) {
	db := &recordingDB{}
	if err := Apply(context.Background(), db, 768); !errors.Is(err, ErrUnsupportedDimensions) {
		t.Fatalf("Apply error = %v", err)
	}
	if db.calls != 0 {
		t.Fatalf("sent %d SQL statements", db.calls)
	}
}

func TestSchemaSHA256_Stable(t *testing.T) {
	first, err := SchemaSHA256(1024)
	if err != nil {
		t.Fatal(err)
	}
	second, err := SchemaSHA256(1024)
	if err != nil {
		t.Fatal(err)
	}
	if first != second || len(first) != 64 {
		t.Fatalf("unstable hash: %q %q", first, second)
	}
	if _, err := SchemaSHA256(768); !errors.Is(err, ErrUnsupportedDimensions) {
		t.Fatalf("error = %v", err)
	}
}

type capturingDB struct{ sql []string }

func (db *capturingDB) Exec(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
	db.sql = append(db.sql, sql)
	return pgconn.CommandTag{}, nil
}

func TestApplyTables_DefersANNIndex(t *testing.T) {
	db := &capturingDB{}
	if err := ApplyTables(context.Background(), db, 1024); err != nil {
		t.Fatal(err)
	}
	if len(db.sql) != 1 || strings.Contains(db.sql[0], "hnsw") {
		t.Fatalf("tables SQL creates the ANN index or is missing: %d statements", len(db.sql))
	}
	if err := ApplyIndexes(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	if len(db.sql) != 2 || !strings.Contains(db.sql[1], "USING hnsw (embedding vector_cosine_ops) WITH (m=16,ef_construction=64)") {
		t.Fatalf("index SQL = %q", db.sql[len(db.sql)-1])
	}
}

func TestSchema_Version4Constraints(t *testing.T) {
	if SchemaVersion != 4 {
		t.Fatalf("schema version = %d, want 4", SchemaVersion)
	}
	for _, part := range []string{"validity_decision", "validity_conflict", "derived_from_text", "manual_correction", "schema_version = 4", "text_source text NOT NULL", "text_complete boolean NOT NULL", "locator text", "text_source <> 'none' OR NOT text_complete"} {
		if !strings.Contains(schema, part) {
			t.Errorf("schema missing %q", part)
		}
	}
}
