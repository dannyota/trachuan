package frameworks

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

type recordingDB struct {
	statements []string
	failAt     int
	err        error
}

func (db *recordingDB) Exec(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
	db.statements = append(db.statements, sql)
	if len(db.statements) == db.failAt {
		return pgconn.CommandTag{}, db.err
	}
	return pgconn.CommandTag{}, nil
}

func TestApply_RejectsUnsupportedDimensions(t *testing.T) {
	for _, dimensions := range []int{0, 768, 1536} {
		db := &recordingDB{}
		if err := Apply(context.Background(), db, dimensions); !errors.Is(err, ErrUnsupportedDimensions) {
			t.Errorf("Apply(%d) = %v", dimensions, err)
		}
		if len(db.statements) != 0 {
			t.Fatal("unsupported dimensions reached database")
		}
		if _, err := SchemaSHA256(dimensions); !errors.Is(err, ErrUnsupportedDimensions) {
			t.Errorf("hash(%d) = %v", dimensions, err)
		}
	}
}

func TestApply_RendersBothIndexesWithoutANN(t *testing.T) {
	db := &recordingDB{}
	if err := Apply(context.Background(), db, 1024); err != nil {
		t.Fatal(err)
	}
	if len(db.statements) != 2 {
		t.Fatalf("statements = %d", len(db.statements))
	}
	if strings.Contains(db.statements[0], "{{") || strings.Count(db.statements[0], "vector(1024)") != 2 {
		t.Fatal("body and public vectors were not rendered")
	}
	sql := strings.Join(db.statements, "")
	if strings.Contains(sql, "hnsw") || strings.Contains(sql, "ivfflat") {
		t.Fatal("exact scan schema created ANN")
	}
	sum := sha256.Sum256([]byte(sql))
	got, err := SchemaSHA256(1024)
	if err != nil || got != hex.EncodeToString(sum[:]) {
		t.Fatalf("hash = %q, error = %v", got, err)
	}
}

func TestApply_WrapsDatabaseErrors(t *testing.T) {
	failure := errors.New("synthetic database failure")
	for _, failAt := range []int{1, 2} {
		db := &recordingDB{failAt: failAt, err: failure}
		if err := Apply(context.Background(), db, 1024); !errors.Is(err, failure) {
			t.Fatalf("Apply error = %v", err)
		}
		if len(db.statements) != failAt {
			t.Fatalf("continued after error: %d statements", len(db.statements))
		}
	}
}

func TestApplyTables_DefersIndexes(t *testing.T) {
	db := &recordingDB{}
	if err := ApplyTables(context.Background(), db, 1024); err != nil {
		t.Fatal(err)
	}
	if len(db.statements) != 1 || strings.Contains(db.statements[0], "CREATE INDEX") {
		t.Fatal("tables created serving indexes")
	}
	if err := ApplyIndexes(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	if len(db.statements) != 2 || !strings.Contains(db.statements[1], "CREATE INDEX") {
		t.Fatal("indexes were not applied separately")
	}
}
