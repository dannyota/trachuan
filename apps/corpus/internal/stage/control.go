package stage

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"danny.vn/trachuan/apps/corpus/internal/store"
)

// StageInput identifies a stage of a run generation.
type StageInput struct {
	RunID      int64
	Generation int
	Stage      string
}

// PlanResult lists the batch numbers that still hold pending items.
type PlanResult struct {
	Batches    []int
	Cancelling bool
}

// CloseResult reports a closed stage.
type CloseResult struct {
	Counts     Counts
	NextStage  string
	Failed     bool
	Cancelling bool
	Reason     string
}

// MarkInput asks for a fenced run state change.
type MarkInput struct {
	RunID       int64
	Generation  int
	From        []string
	To, Message string
}

// PlanStage checks the generation and recipes, then records the stage's items.
func PlanStage(ctx context.Context, env Env, st Stage, in StageInput, compiled map[string]string) (PlanResult, error) {
	var result PlanResult
	tx, err := env.DB.Begin(ctx)
	if err != nil {
		return result, fmt.Errorf("begin stage plan: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := store.New(tx)
	row, err := q.GetControlRunForUpdate(ctx, in.RunID)
	if err != nil {
		return result, fmt.Errorf("lock run for planning: %w", err)
	}
	if int(row.Generation) != in.Generation {
		return result, ErrSuperseded
	}
	if row.State == "cancelling" || row.State == "cancelled" {
		return PlanResult{Cancelling: true}, nil
	}
	var snapshot Snapshot
	if decodeErr := json.Unmarshal(row.Snapshot, &snapshot); decodeErr != nil {
		return result, fmt.Errorf("decode run snapshot: %w", decodeErr)
	}
	if _, ok := snapshot.Recipes[in.Stage]; !ok {
		return result, fmt.Errorf("missing frozen recipe for stage %s", in.Stage)
	}
	for name, recipe := range compiled {
		if frozen, ok := snapshot.Recipes[name]; ok && frozen != recipe {
			return result, fmt.Errorf("recipe mismatch for stage %s", name)
		}
	}
	run := Run{ID: row.ID, Country: row.Country, Rel: row.Rel, Generation: int(row.Generation), Snapshot: snapshot}
	items, err := st.Plan(ctx, env, run)
	if err != nil {
		return result, fmt.Errorf("plan stage %s: %w", in.Stage, err)
	}
	recipe := st.Recipe(snapshot)
	for i, item := range items {
		var inputHash, message *string
		if item.InputHash != "" {
			inputHash = &item.InputHash
		}
		if item.Skip != nil {
			data, marshalErr := json.Marshal(struct {
				Skip struct {
					Kind   string `json:"kind"`
					Detail string `json:"detail"`
				} `json:"skip"`
			}{Skip: struct {
				Kind   string `json:"kind"`
				Detail string `json:"detail"`
			}{Kind: item.Skip.Kind, Detail: item.Skip.Detail}})
			if marshalErr != nil {
				return result, fmt.Errorf("encode skip for %q: %w", item.Key, marshalErr)
			}
			value := string(data)
			message = &value
		}
		if insertErr := q.InsertPlannedItem(ctx, store.InsertPlannedItemParams{RunID: in.RunID, Stage: in.Stage, ItemKey: item.Key, InputHash: inputHash, RecipeHash: &recipe, Batch: int32(i / 100), Message: message}); insertErr != nil {
			return result, fmt.Errorf("insert planned item %q: %w", item.Key, insertErr)
		}
	}
	if resetErr := q.ResetOlderFailedStageItems(ctx, store.ResetOlderFailedStageItemsParams{RunID: in.RunID, Stage: in.Stage, Generation: int32(in.Generation)}); resetErr != nil {
		return result, fmt.Errorf("reset failed stage items: %w", resetErr)
	}
	batches, err := q.ListPendingStageBatches(ctx, store.ListPendingStageBatchesParams{RunID: in.RunID, Stage: in.Stage})
	if err != nil {
		return result, fmt.Errorf("list pending batches: %w", err)
	}
	if commitErr := tx.Commit(ctx); commitErr != nil {
		return result, fmt.Errorf("commit stage plan: %w", commitErr)
	}
	for _, batch := range batches {
		result.Batches = append(result.Batches, int(batch))
	}
	return result, nil
}

// CloseStage records the stage's counts and moves next_stage when no item is pending or failed.
func CloseStage(ctx context.Context, env Env, st Stage, in StageInput, following string) (CloseResult, error) {
	var result CloseResult
	tx, err := env.DB.Begin(ctx)
	if err != nil {
		return result, fmt.Errorf("begin stage close: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := store.New(tx)
	row, err := q.GetControlRunForUpdate(ctx, in.RunID)
	if err != nil {
		return result, fmt.Errorf("lock run for close: %w", err)
	}
	if int(row.Generation) != in.Generation {
		return result, ErrSuperseded
	}
	if row.State == "cancelling" || row.State == "cancelled" {
		return CloseResult{Cancelling: true}, nil
	}
	finalizer, hasFinalizer := st.(Finalizer)
	failures, err := q.ListRunFailures(ctx, in.RunID)
	if err != nil {
		return result, fmt.Errorf("list run failures: %w", err)
	}
	var priorReason string
	for _, failure := range failures {
		for _, code := range BlockingFailureCodes() {
			if failure.Code == code {
				priorReason = failure.Reason
				break
			}
		}
		if priorReason != "" {
			break
		}
	}
	alreadyClosed := hasFinalizer && row.NextStage != nil && *row.NextStage == following && *row.NextStage != in.Stage
	if hasFinalizer && priorReason == "" && !alreadyClosed {
		if row.NextStage == nil || *row.NextStage != in.Stage || st.Name() != in.Stage {
			return result, fmt.Errorf("stage %s is not current", in.Stage)
		}
	}
	items, err := q.ListRunItemCounts(ctx, in.RunID)
	if err != nil {
		return result, fmt.Errorf("count stage items: %w", err)
	}
	result.Counts.UnacceptedGaps = make(map[string]int)
	for _, item := range items {
		if item.Stage != in.Stage {
			continue
		}
		count := int(item.Count)
		result.Counts.Expected += count
		switch item.State {
		case "done":
			result.Counts.Done = count
		case "gap":
			result.Counts.Gap = count
		case "failed":
			result.Counts.Failed = count
		case "pending":
			result.Counts.Pending = count
		}
	}
	gaps, err := q.ListUnacceptedGapCounts(ctx, in.RunID)
	if err != nil {
		return result, fmt.Errorf("count stage gaps: %w", err)
	}
	for _, gap := range gaps {
		if gap.Stage == in.Stage {
			result.Counts.UnacceptedGaps[gap.Kind] = int(gap.Count)
		}
	}
	if priorReason != "" {
		result.Failed, result.Reason = true, priorReason
		return result, nil
	}
	sources, err := q.ListRunSources(ctx, in.RunID)
	if err != nil {
		return result, fmt.Errorf("list run sources: %w", err)
	}
	var paused []string
	for _, source := range sources {
		if source.State == "source_paused" {
			paused = append(paused, source.Source)
		}
	}
	if len(paused) > 0 {
		reason := "source_paused: " + strings.Join(paused, ", ")
		if err = q.SetRunState(ctx, store.SetRunStateParams{ID: in.RunID, State: "failed", Message: &reason}); err != nil {
			return result, fmt.Errorf("mark paused run failed: %w", err)
		}
		if err = tx.Commit(ctx); err != nil {
			return result, fmt.Errorf("commit paused run failure: %w", err)
		}
		result.Failed, result.Reason = true, reason
		return result, nil
	}
	if alreadyClosed {
		result.NextStage = following
		return result, nil
	}
	if result.Counts.Pending > 0 {
		return result, fmt.Errorf("stage %s has %d pending items", in.Stage, result.Counts.Pending)
	}
	if result.Counts.Failed > 0 {
		result.Failed = true
		return result, nil
	}
	if hasFinalizer {
		var snapshot Snapshot
		if err = json.Unmarshal(row.Snapshot, &snapshot); err != nil {
			return result, fmt.Errorf("decode run snapshot: %w", err)
		}
		run := Run{ID: row.ID, Country: row.Country, Rel: row.Rel, Generation: int(row.Generation), Snapshot: snapshot}
		final, finalizeErr := finalizer.Finalize(ctx, tx, run)
		if finalizeErr != nil {
			return result, fmt.Errorf("finalize stage %s: %w", in.Stage, finalizeErr)
		}
		if final.Code != "" {
			if err = recordFinalization(ctx, q, in, final); err != nil {
				return result, err
			}
			if err = tx.Commit(ctx); err != nil {
				return result, fmt.Errorf("commit stage failure: %w", err)
			}
			result.Failed, result.Reason = true, final.Reason
			return result, nil
		}
		gaps, err = q.ListUnacceptedGapCounts(ctx, in.RunID)
		if err != nil {
			return result, fmt.Errorf("count finalized stage gaps: %w", err)
		}
		result.Counts.UnacceptedGaps = make(map[string]int)
		for _, gap := range gaps {
			if gap.Stage == in.Stage {
				result.Counts.UnacceptedGaps[gap.Kind] = int(gap.Count)
			}
		}
	}
	next := &following
	if advanceErr := q.AdvanceRunStage(ctx, store.AdvanceRunStageParams{ID: in.RunID, NextStage: next}); advanceErr != nil {
		return result, fmt.Errorf("advance stage: %w", advanceErr)
	}
	if commitErr := tx.Commit(ctx); commitErr != nil {
		return result, fmt.Errorf("commit stage close: %w", commitErr)
	}
	result.NextStage = following
	return result, nil
}

// MarkRun changes the run state only from an allowed state and only for its own generation.
func MarkRun(ctx context.Context, env Env, in MarkInput) (bool, error) {
	tx, err := env.DB.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("begin run state change: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := store.New(tx)
	row, err := q.GetControlRunForUpdate(ctx, in.RunID)
	if err != nil {
		return false, fmt.Errorf("lock run generation: %w", err)
	}
	if int(row.Generation) != in.Generation {
		return false, ErrSuperseded
	}
	rows, err := q.MarkRunFromStates(ctx, store.MarkRunFromStatesParams{ID: in.RunID, Generation: int32(in.Generation), AllowedStates: in.From, State: in.To, Message: &in.Message})
	if err != nil {
		return false, fmt.Errorf("mark run: %w", err)
	}
	if commitErr := tx.Commit(ctx); commitErr != nil {
		return false, fmt.Errorf("commit run state change: %w", commitErr)
	}
	return rows > 0, nil
}
