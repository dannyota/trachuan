package embed

import (
	"context"
	"fmt"
	"io"

	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/store"
)

// Backfill indexes verified artifacts for the current embed recipe.
func (s *Stage) Backfill(ctx context.Context, env stage.Env, out io.Writer) error {
	recipe := s.Recipe(stage.Snapshot{})
	artifacts, err := store.New(env.DB).ListEmbedArtifacts(ctx, recipe)
	if err != nil {
		return fmt.Errorf("list embed artifacts: %w", err)
	}
	verified, skipped := 0, 0
	var inserted int64
	for _, a := range artifacts {
		cached := s.openCached(env, stage.Run{}, store.LookupEmbedRowsRow{ArtifactID: a.ID, InputHash: a.InputHash, Path: a.Path, Sha256: a.Sha256})
		if cached.err != nil {
			skipped++
			_, _ = fmt.Fprintf(out, "artifact %d skipped: %v\n", a.ID, cached.err)
			continue
		}
		verified++
		tx, beginErr := env.DB.Begin(ctx)
		if beginErr != nil {
			return fmt.Errorf("begin embed index: %w", beginErr)
		}
		count, insertErr := insertRows(ctx, store.New(tx), recipe, a.ID, cached.index.Rows)
		if insertErr != nil {
			_ = tx.Rollback(ctx)
			return insertErr
		}
		if err = tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit embed index: %w", err)
		}
		inserted += count
	}
	_, _ = fmt.Fprintf(out, "artifacts verified=%d rows inserted=%d artifacts skipped=%d\n", verified, inserted, skipped)
	return nil
}
