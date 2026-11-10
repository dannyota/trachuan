package discover

import (
	"context"
	"errors"
	"fmt"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/store"
	"github.com/jackc/pgx/v5"
)

type secondaryHistory struct{ present map[string]bool }

func earlierSecondary(ctx context.Context, q *store.Queries, env stage.Env, rows []store.ListEarlierRunsRow, current map[string]secondaryObservation) (map[string]secondaryHistory, error) {
	out := map[string]secondaryHistory{}
	for key, now := range current {
		for _, run := range rows {
			row, err := q.GetRunItemArtifact(ctx, store.GetRunItemArtifactParams{RunID: run.ID, Stage: "discover", ItemKey: key})
			if errors.Is(err, pgx.ErrNoRows) {
				continue
			}
			if err != nil {
				return nil, fmt.Errorf("read previous secondary item: %w", err)
			}
			if row.State != "done" {
				continue
			}
			dir := env.Artifacts.Abs(row.Path)
			hash, err := artifact.Verify(dir)
			if err != nil {
				return nil, fmt.Errorf("verify previous secondary item: %w", err)
			}
			if hash != row.Sha256 {
				return nil, fmt.Errorf("previous secondary item hash mismatch")
			}
			var report secondaryReport
			if err = readJSON(dir, "secondary.json", &report); err != nil {
				return nil, err
			}
			if report.Version != 1 {
				return nil, fmt.Errorf("unsupported secondary artifact version %d", report.Version)
			}
			if !report.Stable || report.QueryIdentity != now.report.QueryIdentity {
				continue
			}
			out[key] = secondaryHistory{present: secondarySelectedIDs(report)}
			break
		}
	}
	return out, nil
}

func secondarySelectedIDs(report secondaryReport) map[string]bool {
	present := map[string]bool{}
	passes := report.Passes
	if report.Stable && len(passes) > 0 {
		passes = passes[len(passes)-1:]
	}
	for _, pass := range passes {
		for _, item := range pass.Items {
			present[item.SourceID] = true
		}
	}
	return present
}
