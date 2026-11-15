package frameworks

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

const SchemaVersion = 1

var ErrUnsupportedDimensions = errors.New("unsupported vector dimensions")

type DB interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

//go:embed schema.sql
var schema string

//go:embed versions.sql
var versions string

//go:embed evidence.sql
var evidence string

//go:embed indexes.sql
var indexes string

func rendered(dimensions int) (string, error) {
	if dimensions != 1024 {
		return "", fmt.Errorf("dimensions %d: %w", dimensions, ErrUnsupportedDimensions)
	}
	return strings.ReplaceAll(schema, "{{dimensions}}", "1024") + versions + evidence, nil
}

func Apply(ctx context.Context, db DB, dimensions int) error {
	if err := ApplyTables(ctx, db, dimensions); err != nil {
		return err
	}
	return ApplyIndexes(ctx, db)
}

func ApplyTables(ctx context.Context, db DB, dimensions int) error {
	sql, err := rendered(dimensions)
	if err != nil {
		return err
	}
	if _, err := db.Exec(ctx, sql); err != nil {
		return fmt.Errorf("apply frameworks schema (requires vector extension): %w", err)
	}
	return nil
}

func ApplyIndexes(ctx context.Context, db DB) error {
	if _, err := db.Exec(ctx, indexes); err != nil {
		return fmt.Errorf("create frameworks indexes: %w", err)
	}
	return nil
}

func SchemaSHA256(dimensions int) (string, error) {
	sql, err := rendered(dimensions)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(sql + indexes))
	return hex.EncodeToString(sum[:]), nil
}
