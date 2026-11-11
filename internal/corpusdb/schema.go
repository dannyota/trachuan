// Package corpusdb applies the versioned corpus database schema.
package corpusdb

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
)

// SchemaVersion is the version of the corpus schema.
const SchemaVersion = 5

// SupportedSchemaVersions returns the corpus versions supported by the server.
func SupportedSchemaVersions() []int { return []int{4, 5} }

// SparseDimensions is the dimension count of lexical recipe 1.
const SparseDimensions = 1048576

// ErrUnsupportedDimensions identifies an unsupported dense vector dimension.
var ErrUnsupportedDimensions = errors.New("unsupported vector dimensions")

// DB is the database operation needed to apply the schema.
type DB interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

//go:embed schema.sql
var schema string

//go:embed ann.sql
var ann string

func rendered(dimensions int) (string, error) {
	if dimensions != 1024 {
		return "", fmt.Errorf("dimensions %d: %w", dimensions, ErrUnsupportedDimensions)
	}
	return strings.Replace(schema, "{{dimensions}}", "1024", 1), nil
}

// Apply creates the corpus schema, including the ANN index, in a database with
// vector and ltree installed.
func Apply(ctx context.Context, db DB, dimensions int) error {
	if err := ApplyTables(ctx, db, dimensions); err != nil {
		return err
	}
	return ApplyIndexes(ctx, db)
}

// ApplyTables creates every table, constraint and btree or GiST index, but not
// the ANN index, so a build can load rows before paying for HNSW construction.
func ApplyTables(ctx context.Context, db DB, dimensions int) error {
	sql, err := rendered(dimensions)
	if err != nil {
		return err
	}
	if _, err := db.Exec(ctx, sql); err != nil {
		return fmt.Errorf("apply corpus schema (requires vector and ltree extensions): %w", err)
	}
	return nil
}

// ApplyIndexes creates the HNSW index on chunk embeddings.
func ApplyIndexes(ctx context.Context, db DB) error {
	if _, err := db.Exec(ctx, ann); err != nil {
		return fmt.Errorf("create ANN index: %w", err)
	}
	return nil
}

// SchemaSHA256 returns the SHA-256 hash of the rendered schema and ANN index.
func SchemaSHA256(dimensions int) (string, error) {
	sql, err := rendered(dimensions)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(sql + ann))
	return hex.EncodeToString(sum[:]), nil
}
