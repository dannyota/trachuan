package chunk

import (
	"context"
	"encoding/json"
	"fmt"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/store"
	"github.com/jackc/pgx/v5"
)

// Rows prepares workshop rows from the published artifact.
func (*Stage) Rows(ctx context.Context, env stage.Env, art artifact.Result) (func(context.Context, pgx.Tx, int64) error, error) {
	if err := verifyInput(env, art.Path, art.SHA256); err != nil {
		return nil, err
	}
	file, _, err := ReadArtifact(env.Artifacts.Abs(art.Path))
	if err != nil {
		return nil, err
	}
	extractID, err := store.New(env.DB).GetExtractArtifactByHash(ctx, store.GetExtractArtifactByHashParams{Sha256: file.ExtractSHA256, ItemKey: file.DocumentKey})
	if err != nil {
		return nil, fmt.Errorf("resolve extract artifact: %w", err)
	}
	return func(ctx context.Context, tx pgx.Tx, id int64) error {
		q := store.New(tx)
		for _, c := range file.Chunks {
			spans, marshalErr := json.Marshal(c.Spans)
			if marshalErr != nil {
				return fmt.Errorf("encode spans: %w", marshalErr)
			}
			if insertErr := q.InsertChunk(ctx, store.InsertChunkParams{ArtifactID: id, Ordinal: int32(c.Ordinal), ExtractArtifactID: extractID, NodeID: c.NodeID, Anchor: c.Anchor, ContextPrefix: c.ContextPrefix, Content: c.Content, RoughTokens: int32(c.RoughTokens), Spans: spans}); insertErr != nil {
				return fmt.Errorf("insert chunk: %w", insertErr)
			}
		}
		return nil
	}, nil
}

// Resolve recreates the gaps recorded in the artifact.
func (*Stage) Resolve(_ context.Context, env stage.Env, _ stage.Run, _ stage.Item, art artifact.Result) (stage.Resolution, error) {
	if err := verifyInput(env, art.Path, art.SHA256); err != nil {
		return stage.Resolution{}, err
	}
	_, coverage, err := ReadArtifact(env.Artifacts.Abs(art.Path))
	if err != nil {
		return stage.Resolution{}, err
	}
	gaps := make([]stage.Gap, 0, len(coverage.Gaps))
	for _, gap := range coverage.Gaps {
		gaps = append(gaps, stage.Gap{Kind: gap.Kind, Detail: gap.Detail})
	}
	return stage.Resolution{Gaps: gaps}, nil
}
