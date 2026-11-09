package stage

import (
	"context"
	"errors"
	"fmt"

	"danny.vn/traquydinh/apps/corpus/internal/artifact"
	"danny.vn/traquydinh/apps/corpus/internal/store"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Commit holds the rows and fence for one stage item.
type Commit struct {
	RunID        int64
	Stage        string
	ItemKey      string
	Generation   int
	Attempt      int
	RecipeHash   string
	Artifact     *artifact.Result
	ArtifactRows func(context.Context, pgx.Tx, int64) error
	RunRows      func(context.Context, pgx.Tx) error
	Gaps         []Gap
	State        string
	Message      string
}

// CommitItem commits an item and its rows if its fence is current.
func CommitItem(ctx context.Context, pool *pgxpool.Pool, c Commit) (applied bool, err error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("begin item commit: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	q := store.New(tx)
	generation, err := q.GetRunGenerationForShare(ctx, c.RunID)
	if err != nil {
		return false, fmt.Errorf("lock run generation: %w", err)
	}
	if int(generation) != c.Generation {
		return false, nil
	}

	var artifactID *int64
	var inputHash *string
	if c.Artifact != nil {
		inputHash = &c.Artifact.InputHash
		id, insertErr := q.InsertArtifact(ctx, store.InsertArtifactParams{
			Stage: c.Stage, ItemKey: c.ItemKey, InputHash: c.Artifact.InputHash,
			RecipeHash: c.RecipeHash, Path: c.Artifact.Path, Sha256: c.Artifact.SHA256,
		})
		if errors.Is(insertErr, pgx.ErrNoRows) {
			id, err = q.GetArtifactID(ctx, store.GetArtifactIDParams{
				Stage: c.Stage, InputHash: c.Artifact.InputHash, RecipeHash: c.RecipeHash,
			})
			if err != nil {
				return false, fmt.Errorf("get artifact ID: %w", err)
			}
		} else if insertErr != nil {
			return false, fmt.Errorf("insert artifact: %w", insertErr)
		} else if c.ArtifactRows != nil {
			if rowsErr := c.ArtifactRows(ctx, tx, id); rowsErr != nil {
				return false, fmt.Errorf("write artifact rows: %w", rowsErr)
			}
		}
		artifactID = &id
	}
	if c.RunRows != nil {
		if runErr := c.RunRows(ctx, tx); runErr != nil {
			return false, fmt.Errorf("write run rows: %w", runErr)
		}
	}
	for _, gap := range c.Gaps {
		if gapErr := q.InsertGap(ctx, store.InsertGapParams{
			RunID: c.RunID, Stage: c.Stage, ItemKey: c.ItemKey, Kind: gap.Kind, Detail: gap.Detail,
		}); gapErr != nil {
			return false, fmt.Errorf("insert gap: %w", gapErr)
		}
	}
	rows, err := q.UpdateRunItemFenced(ctx, store.UpdateRunItemFencedParams{
		InputHash: inputHash, RecipeHash: &c.RecipeHash, Generation: int32(c.Generation),
		Attempt: int32(c.Attempt), ArtifactID: artifactID, State: c.State, Message: &c.Message,
		RunID: c.RunID, Stage: c.Stage, ItemKey: c.ItemKey,
	})
	if err != nil {
		return false, fmt.Errorf("update run item: %w", err)
	}
	if rows == 0 {
		return false, nil
	}
	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("commit item: %w", err)
	}
	return true, nil
}
