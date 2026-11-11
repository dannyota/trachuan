package stage

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/sourcegate"
	"danny.vn/trachuan/apps/corpus/internal/store"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
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
	// Sources lists every source the item contacted or depends on.
	Sources      []string
	Observations []FetchObservation
	State        string
	Message      string
	// SourceRead distinguishes a fresh read from reuse of committed bytes.
	SourceRead bool
}

// CommitItem commits an item and its rows if its fence is current.
func CommitItem(ctx context.Context, pool *pgxpool.Pool, c Commit) (applied bool, err error) {
	return commitItem(ctx, pool, &c)
}

func commitItem(ctx context.Context, pool *pgxpool.Pool, c *Commit) (applied bool, err error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("begin item commit: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	q := store.New(tx)
	generation, err := q.GetRunGenerationForShare(ctx, c.RunID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("lock run generation: %w", err)
	}
	if int(generation) != c.Generation {
		return false, nil
	}

	item, err := q.GetRunItemCommitted(ctx, store.GetRunItemCommittedParams{RunID: c.RunID, Stage: c.Stage, ItemKey: c.ItemKey})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("lock run item: %w", err)
	}
	if int(item.Generation) > c.Generation || (int(item.Generation) == c.Generation && int(item.Attempt) > c.Attempt) {
		return false, nil
	}
	if int(item.Generation) == c.Generation && int(item.Attempt) == c.Attempt && (item.State == "done" || item.State == "gap" || item.State == "failed") {
		same, compareErr := sameResult(ctx, q, c, item)
		if compareErr != nil {
			return false, compareErr
		}
		if !same {
			return false, fmt.Errorf("conflicting terminal item replay")
		}
		return false, nil
	}
	if c.Stage != "fetch" && len(c.Observations) != 0 {
		return false, fmt.Errorf("observations on non-fetch item")
	}

	for _, source := range c.Sources {
		row, sourceErr := q.GetRunSourceForShare(ctx, store.GetRunSourceForShareParams{RunID: c.RunID, Source: source})
		if sourceErr != nil && !errors.Is(sourceErr, pgx.ErrNoRows) {
			return false, fmt.Errorf("lock source for item commit: %w", sourceErr)
		}
		if sourceErr == nil && row.State != "active" {
			*c = failedCommit(*c, sourcegate.ErrSourcePaused)
			break
		}
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
		} else {
			if c.ArtifactRows != nil {
				if rowsErr := c.ArtifactRows(ctx, tx, id); rowsErr != nil {
					return false, fmt.Errorf("write artifact rows: %w", rowsErr)
				}
			}
			for _, observation := range c.Observations {
				if observation.Name == nil {
					continue
				}
				_, timeErr := q.SetFetchedFileTime(ctx, store.SetFetchedFileTimeParams{ArtifactID: id, Source: observation.Source, Name: *observation.Name, Path: observation.Path, Sha256: observation.ContentSHA256, Size: observation.Size, FetchedAt: observation.FetchedAt, Url: observation.URL})
				if timeErr != nil {
					return false, fmt.Errorf("set fetched file time: %w", timeErr)
				}
			}
		}
		artifactID = &id
	}
	if c.Artifact != nil {
		seen := make(map[string]bool, len(c.Observations))
		for _, o := range c.Observations {
			if seen[o.ResourceKey] {
				return false, fmt.Errorf("duplicate fetch observation resource key")
			}
			seen[o.ResourceKey] = true
			err = q.InsertFetchObservation(ctx, store.InsertFetchObservationParams{RunID: c.RunID, ItemKey: c.ItemKey, Generation: int32(c.Generation), Attempt: int32(c.Attempt), ResourceKey: o.ResourceKey, ArtifactSha256: c.Artifact.SHA256, Source: o.Source, Path: o.Path, Url: o.URL, ContentSha256: o.ContentSHA256, Size: o.Size, Acquisition: o.Acquisition, FetchedAt: o.FetchedAt, OriginRef: o.OriginRef})
			if err != nil {
				return false, fmt.Errorf("insert fetch observation: %w", err)
			}
		}
	}
	if c.RunRows != nil {
		if runErr := c.RunRows(ctx, tx); runErr != nil {
			return false, fmt.Errorf("write run rows: %w", runErr)
		}
	}
	for _, gap := range c.Gaps {
		if validateErr := gap.Validate(); validateErr != nil {
			return false, fmt.Errorf("validate gap: %w", validateErr)
		}
		status := pgtype.Int4{}
		if gap.HTTPStatus != nil {
			status = pgtype.Int4{Int32: *gap.HTTPStatus, Valid: true}
		}
		if gapErr := q.InsertGap(ctx, store.InsertGapParams{
			RunID: c.RunID, Stage: c.Stage, ItemKey: c.ItemKey, Kind: gap.Kind, Detail: gap.Detail,
			Source: optionalString(gap.Source), Origin: optionalString(string(gap.Origin)), Cause: optionalString(string(gap.Cause)), Endpoint: optionalString(gap.Endpoint), HttpStatus: status, EvidenceRef: optionalString(gap.EvidenceRef),
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
	if c.SourceRead && len(c.Sources) > 0 && c.State == "done" && artifactID != nil && (c.Stage == "fetch" || c.Stage == "expand") {
		if err = q.UpsertRunAcquisition(ctx, store.UpsertRunAcquisitionParams{RunID: c.RunID, Stage: c.Stage, DocumentKey: c.ItemKey, ArtifactID: *artifactID, Generation: int32(c.Generation)}); err != nil {
			return false, fmt.Errorf("record acquisition: %w", err)
		}
	}
	parts := strings.SplitN(c.ItemKey, ":", 3)
	if c.Stage == "fetch" && c.State == "done" && artifactID != nil && c.Message != AcquisitionMessage && len(parts) == 3 && parts[0] != "" && parts[1] == "vbpl" && parts[2] != "" {
		if err = q.UpsertFetchBaseline(ctx, store.UpsertFetchBaselineParams{DocumentKey: c.ItemKey, FetchRecipeHash: c.RecipeHash, ArtifactID: *artifactID, RunID: c.RunID, ItemKey: c.ItemKey, Generation: int32(c.Generation), Attempt: int32(c.Attempt)}); err != nil {
			return false, fmt.Errorf("record fetch baseline: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("commit item: %w", err)
	}
	return true, nil
}

func optionalString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func sameResult(ctx context.Context, q *store.Queries, c *Commit, item store.GetRunItemCommittedRow) (bool, error) {
	if item.State != c.State || item.Message == nil || *item.Message != c.Message {
		return false, nil
	}
	var hash *string
	if c.Artifact != nil {
		hash = &c.Artifact.SHA256
	}
	if !sameString(item.ArtifactSha256, hash) {
		return false, nil
	}
	if c.Stage != "fetch" {
		return len(c.Observations) == 0, nil
	}
	rows, err := q.ListItemFetchObservations(ctx, store.ListItemFetchObservationsParams{RunID: c.RunID, ItemKey: c.ItemKey, Generation: int32(c.Generation), Attempt: int32(c.Attempt)})
	if err != nil {
		return false, fmt.Errorf("list fetch observations: %w", err)
	}
	if len(rows) != len(c.Observations) {
		return false, nil
	}
	byKey := make(map[string]FetchObservation, len(c.Observations))
	for _, o := range c.Observations {
		if _, found := byKey[o.ResourceKey]; found {
			return false, nil
		}
		byKey[o.ResourceKey] = o
	}
	for _, row := range rows {
		o, ok := byKey[row.ResourceKey]
		if !ok || hash == nil || row.ArtifactSha256 != *hash || row.Source != o.Source || row.Path != o.Path || !sameString(row.Url, o.URL) || row.ContentSha256 != o.ContentSHA256 || row.Size != o.Size || row.Acquisition != o.Acquisition || !sameTime(row.FetchedAt, o.FetchedAt) || !sameString(row.OriginRef, o.OriginRef) {
			return false, nil
		}
	}
	return true, nil
}

func sameString(a, b *string) bool { return a == nil && b == nil || a != nil && b != nil && *a == *b }
func sameTime(a, b *time.Time) bool {
	return a == nil && b == nil || a != nil && b != nil && a.Equal(b.Truncate(time.Microsecond))
}
