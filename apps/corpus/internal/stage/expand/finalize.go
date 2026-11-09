package expand

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/stage/discover"
	"danny.vn/trachuan/apps/corpus/internal/stage/membership"
	"danny.vn/trachuan/apps/corpus/internal/store"
	"github.com/jackc/pgx/v5"
)

// Finalize freezes expanded membership in the close transaction. Current and
// retained tuples belong to their seed's expand item; removed tuples from a
// departed seed belong to the current discovery central item. A frozen phase
// accepts only identical recomputed membership, evidence and observations;
// item-state changes with identical outputs do not alter the frozen result.
func (*Stage) Finalize(ctx context.Context, env stage.Env, tx pgx.Tx, r stage.Run) (stage.Finalization, error) {
	q := store.New(tx)
	seeds, err := membership.ReadPhase(ctx, q, r.ID, membership.Seeds)
	if err != nil {
		return stage.Finalization{}, fmt.Errorf("read seeds: %w", err)
	}
	items, err := q.ListStageItemArtifacts(ctx, store.ListStageItemArtifactsParams{RunID: r.ID, Stage: "expand"})
	if err != nil {
		return stage.Finalization{}, fmt.Errorf("list expand items: %w", err)
	}
	planned := seedItems(seeds.Entries)
	if len(items) != len(planned) {
		return stage.Finalization{}, fmt.Errorf("expand items do not match seeds")
	}
	observations := make(map[string]seedObservation, len(items))
	stored := make([]store.UpsertSeedObservationParams, 0, len(items))
	for i, item := range items {
		if item.ItemKey != planned[i].Key {
			return stage.Finalization{}, fmt.Errorf("expand item %q does not match seed %q", item.ItemKey, planned[i].Key)
		}
		o, readErr := readSeedObservation(env, item)
		if readErr != nil {
			return stage.Finalization{}, readErr
		}
		observations[item.ItemKey] = o
		targets := []string{}
		if o.complete {
			seen := map[string]bool{}
			for _, t := range o.observation.Targets {
				seen[t.TargetKey] = true
			}
			for key := range seen {
				targets = append(targets, key)
			}
			sort.Strings(targets)
		}
		data, encodeErr := json.Marshal(targets)
		if encodeErr != nil {
			return stage.Finalization{}, fmt.Errorf("encode targets: %w", encodeErr)
		}
		completeness := "incomplete"
		if o.complete {
			completeness = "complete"
		}
		stored = append(stored, store.UpsertSeedObservationParams{RunID: r.ID, SeedKey: item.ItemKey, ItemKey: item.ItemKey, Completeness: completeness, ArtifactHash: o.artifactHash, ObservedTargets: data})
	}
	prior, err := discover.EarlierMembership(ctx, q, r)
	if err != nil {
		return stage.Finalization{}, fmt.Errorf("read earlier membership: %w", err)
	}
	members := reconcile(seeds.Entries, prior, observations)
	entries := make([]membership.Entry, 0, len(members))
	records := []membership.EvidenceRecord{}
	for _, m := range members {
		entries = append(entries, m.entry)
		records = append(records, m.records...)
	}
	frozen, err := verifyFrozen(ctx, q, r.ID, stored, records)
	if err != nil {
		return stage.Finalization{}, err
	}
	if err = membership.PublishPhase(ctx, q, r.ID, membership.Expanded, entries); err != nil {
		return stage.Finalization{}, fmt.Errorf("publish expanded membership: %w", err)
	}
	if frozen {
		return stage.Finalization{}, nil
	}
	for _, m := range members {
		if err = membership.UpsertDocument(ctx, q, r.ID, m.entry); err != nil {
			return stage.Finalization{}, err
		}
		for _, rec := range m.records {
			if err = membership.RecordEvidence(ctx, q, r.ID, rec); err != nil {
				return stage.Finalization{}, err
			}
		}
	}
	for _, o := range stored {
		if err = q.UpsertSeedObservation(ctx, o); err != nil {
			return stage.Finalization{}, fmt.Errorf("store seed observation: %w", err)
		}
	}
	return stage.Finalization{}, nil
}
func readSeedObservation(env stage.Env, item store.ListStageItemArtifactsRow) (seedObservation, error) {
	if item.State != "done" || item.Path == nil || item.Sha256 == nil {
		return seedObservation{}, nil
	}
	dir := env.Artifacts.Abs(*item.Path)
	hash, err := artifact.Verify(dir)
	if err != nil {
		return seedObservation{}, fmt.Errorf("verify expand artifact %s: %w", item.ItemKey, err)
	}
	if hash != *item.Sha256 {
		return seedObservation{}, fmt.Errorf("expand artifact hash mismatch for %s", item.ItemKey)
	}
	data, err := os.ReadFile(filepath.Join(dir, "observation.json"))
	if err != nil {
		return seedObservation{}, fmt.Errorf("read expand observation: %w", err)
	}
	var obs Observation
	if err = json.Unmarshal(data, &obs); err != nil {
		return seedObservation{}, fmt.Errorf("decode expand observation: %w", err)
	}
	if obs.Version != 1 || obs.SeedKey != item.ItemKey {
		return seedObservation{}, fmt.Errorf("invalid expand observation for %s", item.ItemKey)
	}
	for _, t := range obs.Targets {
		if t.Response != "detail.json" && t.Response != "diagram.json" {
			return seedObservation{}, fmt.Errorf("invalid expand response path %q", t.Response)
		}
	}
	return seedObservation{complete: true, artifactHash: item.Sha256, observation: obs}, nil
}
