package build

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var errInvalidDatabase = errors.New("invalid corpus database")

func incompleteDatabase(err error) bool {
	var pgErr *pgconn.PgError
	return errors.Is(err, errInvalidDatabase) || errors.Is(err, pgx.ErrNoRows) || (errors.As(err, &pgErr) && pgErr.Code == "42P01")
}

func validateDatabase(ctx context.Context, cfg Config, name string, record BuildRecord) error {
	c, err := pgx.ParseConfig(cfg.BuilderURL)
	if err != nil {
		return fmt.Errorf("parse builder configuration: %w", err)
	}
	c.Database = name
	db, err := pgx.ConnectConfig(ctx, c)
	if err != nil {
		return fmt.Errorf("connect corpus database: %w", err)
	}
	defer func() { _ = db.Close(context.Background()) }()
	var matches bool
	err = db.QueryRow(ctx, `SELECT country=$1 AND rel=$2 AND schema_version=$3 AND taxonomy_version=$4 AND dense_recipe=$5::jsonb AND chunk_recipe=$6::jsonb AND lexical_recipe=$7::jsonb AND retrieval_recipe=$8::jsonb AND lexical_stats=$9::jsonb AND source_manifest_hash=$10 AND build_input_hash=$11 AND build_recipe_hash=$12 AND evaluation_summary=$14::jsonb AND built_at=$13 FROM release WHERE singleton`, record.Country, record.Rel, record.SchemaVersion, record.TaxonomyVersion, record.DenseRecipe, record.ChunkRecipe, record.LexicalRecipe, record.RetrievalRecipe, record.LexicalStats, record.SourceManifestHash, record.BuildInputHash, record.BuildRecipeHash, record.BuiltAt, initialEvaluationSummary()).Scan(&matches)
	if err != nil {
		return fmt.Errorf("read corpus release: %w", err)
	}
	if !matches {
		return fmt.Errorf("release mismatch: %w", errInvalidDatabase)
	}
	for _, table := range []string{"document", "source_file", "provision", "provision_validity", "chunk", "lexical_df", "relation", "coverage_gap", "taxonomy_node", "taxonomy_assignment", "release"} {
		expected, ok := record.Counts[table]
		if !ok {
			return fmt.Errorf("missing corpus count for %s: %w", table, errInvalidDatabase)
		}
		var actual int64
		if err = db.QueryRow(ctx, "SELECT count(*) FROM "+pgx.Identifier{table}.Sanitize()).Scan(&actual); err != nil {
			return fmt.Errorf("count corpus table %s: %w", table, err)
		}
		if actual != expected {
			return fmt.Errorf("corpus count mismatch for %s: %w", table, errInvalidDatabase)
		}
	}
	if err = db.QueryRow(ctx, `SELECT NOT EXISTS (SELECT 1 FROM provision p JOIN document d ON d.id=p.document_id WHERE (p.locator IS NULL) <> (d.text_source='source_tree'))`).Scan(&matches); err != nil {
		return fmt.Errorf("validate provision locators: %w", err)
	}
	if !matches {
		return fmt.Errorf("invalid provision locator: %w", errInvalidDatabase)
	}
	if err = validateTaxonomy(ctx, db, record); err != nil {
		return err
	}
	return nil
}

func validateLocked(ctx context.Context, admin *pgx.Conn, cfg Config, name string, record BuildRecord) error {
	work, cancel := context.WithCancel(ctx)
	defer cancel()
	stop := watchLock(work, admin, cancel, nil)
	err := validateDatabase(work, cfg, name, record)
	watchErr := stop()
	if watchErr != nil {
		return watchErr
	}
	if pingErr := admin.Ping(ctx); pingErr != nil {
		return fmt.Errorf("build lock lost: %w", pingErr)
	}
	return err
}
