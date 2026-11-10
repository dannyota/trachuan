package run

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/stage/membership"
	"danny.vn/trachuan/apps/corpus/internal/store"
	"github.com/jackc/pgx/v5"
)

type retryItem struct{ stage, key string }
type retryPlan struct {
	items  []retryItem
	rewind int
	stages []string
}

func planRetry(ctx context.Context, q *store.Queries, runID int64, stages []string) (retryPlan, error) {
	gaps, err := q.ListRunGapsByRun(ctx, runID)
	if err != nil {
		return retryPlan{}, fmt.Errorf("list retry gaps: %w", err)
	}
	eligible := make(map[retryItem]bool)
	for _, g := range gaps {
		item := retryItem{g.Stage, g.ItemKey}
		if _, ok := eligible[item]; !ok {
			eligible[item] = true
		}
		gap := stage.Gap{Kind: g.Kind}
		if g.Origin != nil {
			gap.Origin = stage.Origin(*g.Origin)
		}
		if g.Cause != nil {
			gap.Cause = stage.Cause(*g.Cause)
		}
		if gap.Validate() != nil || !stage.RetryEligible(gap) {
			eligible[item] = false
		}
	}
	plan := retryPlan{rewind: len(stages)}
	for i, name := range stages {
		has := false
		for _, g := range gaps {
			item := retryItem{g.Stage, g.ItemKey}
			if g.Stage != name || !eligible[item] {
				continue
			}
			delete(eligible, item)
			if name == "discover" && !strings.HasPrefix(item.key, "chinhphu:") {
				_, artifactErr := q.GetRunItemArtifact(ctx, store.GetRunItemArtifactParams{RunID: runID, Stage: name, ItemKey: item.key})
				if artifactErr == nil {
					return retryPlan{}, fmt.Errorf("discover item with an artifact cannot be retried: %s", item.key)
				}
				if !errors.Is(artifactErr, pgx.ErrNoRows) {
					return retryPlan{}, fmt.Errorf("check discover artifact: %w", artifactErr)
				}
			}
			plan.items = append(plan.items, item)
			has = true
		}
		if has {
			if plan.rewind == len(stages) {
				plan.rewind = i
			}
			plan.stages = append(plan.stages, name)
		}
	}
	for item, ok := range eligible {
		if ok {
			return retryPlan{}, fmt.Errorf("retry item %s has unknown stage %q", item.key, item.stage)
		}
	}
	return plan, nil
}

func applyRetry(ctx context.Context, q *store.Queries, runID int64, generation int32, stages []string, plan retryPlan) error {
	for _, item := range plan.items {
		if _, err := q.DeleteItemGaps(ctx, store.DeleteItemGapsParams{RunID: runID, Stage: item.stage, ItemKey: item.key}); err != nil {
			return fmt.Errorf("delete retry gaps: %w", err)
		}
		n, err := q.ResetRetryItem(ctx, store.ResetRetryItemParams{RunID: runID, Stage: item.stage, ItemKey: item.key, Generation: generation})
		if err != nil {
			return fmt.Errorf("reset retry item: %w", err)
		}
		if n != 1 {
			return fmt.Errorf("retry item %s/%s missing", item.stage, item.key)
		}
	}
	later := stages[plan.rewind+1:]
	if _, err := q.DeleteStageGaps(ctx, store.DeleteStageGapsParams{RunID: runID, Stages: later}); err != nil {
		return fmt.Errorf("delete later gaps: %w", err)
	}
	affected := stages[plan.rewind:]
	var kinds []string
	if containsStage(affected, "expand") {
		kinds = []string{"relation"}
	}
	// Clear finalizer evidence at and after the rewind so finalizers rebuild it
	// from preserved artifacts without retaining stale support tuples.
	if _, err := q.DeleteStageMembershipEvidence(ctx, store.DeleteStageMembershipEvidenceParams{RunID: runID, Stages: affected, RuleKinds: kinds}); err != nil {
		return fmt.Errorf("delete membership evidence: %w", err)
	}
	if _, err := q.DeleteStageSeedObservations(ctx, store.DeleteStageSeedObservationsParams{RunID: runID, Stages: affected}); err != nil {
		return fmt.Errorf("delete seed observations: %w", err)
	}
	if _, err := q.DeleteStageItems(ctx, store.DeleteStageItemsParams{RunID: runID, Stages: later}); err != nil {
		return fmt.Errorf("delete later items: %w", err)
	}
	if _, err := q.DeleteUnsupportedRunDocuments(ctx, runID); err != nil {
		return fmt.Errorf("delete unsupported documents: %w", err)
	}
	for _, p := range []struct {
		name  string
		phase membership.Phase
	}{{"discover", membership.Seeds}, {"expand", membership.Expanded}, {"fetch", membership.Final}} {
		if containsStage(affected, p.name) {
			if err := membership.ClearPhase(ctx, q, runID, p.phase); err != nil {
				return fmt.Errorf("clear retry phase: %w", err)
			}
		}
	}
	return nil
}

func (p retryPlan) message(stages []string) string {
	return fmt.Sprintf("retry: %d items reset in %s, rewind to %s", len(p.items), strings.Join(p.stages, ", "), stages[p.rewind])
}
