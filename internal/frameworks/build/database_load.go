package build

import (
	"context"
	"fmt"

	schema "danny.vn/trachuan/internal/corpusdb/frameworks"
	"danny.vn/trachuan/internal/embedding"
	"danny.vn/trachuan/internal/frameworks/relate"
	"github.com/jackc/pgx/v5"
)

type databaseIDs struct {
	versions map[string]int64
	controls map[relate.Key]int64
}

func loadDatabase(ctx context.Context, conn *pgx.Conn, base, rel string, d databaseInputs, embedder Embedder) error {
	if err := schema.ApplyTables(ctx, conn, embedding.DocumentRecipe1().Dimensions); err != nil {
		return hide("apply frameworks tables", err)
	}
	tx, err := conn.Begin(ctx)
	if err != nil {
		return hide("begin frameworks load", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if err = initializeRelease(ctx, tx, rel, d.steps); err != nil {
		return err
	}
	ids := databaseIDs{versions: map[string]int64{}, controls: map[relate.Key]int64{}}
	frameworkCount, err := loadVersions(ctx, tx, d, &ids)
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE release SET versions_sealed=true`); err != nil {
		return hide("seal framework versions", err)
	}
	if err = loadControls(ctx, tx, d, &ids); err != nil {
		return err
	}
	if err = loadSourceFiles(ctx, tx, d, ids); err != nil {
		return err
	}
	if err = loadChunks(ctx, tx, base, d, ids, embedder); err != nil {
		return err
	}
	if err = loadEdges(ctx, tx, d, ids); err != nil {
		return err
	}
	if err = loadGaps(ctx, tx, d, ids); err != nil {
		return err
	}
	if err = schema.ApplyIndexes(ctx, tx); err != nil {
		return hide("apply frameworks indexes", err)
	}
	if _, err = tx.Exec(ctx, `UPDATE release SET framework_count=$1,version_count=$2,control_count=$3`, frameworkCount, len(d.versions), len(d.controls)); err != nil {
		return hide("finalize frameworks release", err)
	}
	if err = tx.Commit(ctx); err != nil {
		return hide("commit frameworks load", err)
	}
	return nil
}

func versionKey(code, version string) string { return code + ":" + version }

func requiredID[K comparable](ids map[K]int64, key K) (int64, error) {
	id, ok := ids[key]
	if !ok {
		return 0, fmt.Errorf("unknown database identity")
	}
	return id, nil
}
