package fetch

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/source"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/stage/expand"
	"danny.vn/trachuan/apps/corpus/internal/stage/membership"
	"danny.vn/trachuan/apps/corpus/internal/store"
	"github.com/jackc/pgx/v5"
)

type drift struct{ seed, target, code, reason, expandHash, fetchHash string }

func findDrift(seed string, obs expand.Observation, s membership.PhaseSnapshot, expandHash, fetchHash string) []drift {
	active := map[string]bool{}
	removed := map[string]bool{}
	for _, e := range s.Active {
		active[e.DocumentKey] = true
	}
	for _, e := range s.RetainedRemovals {
		removed[e.DocumentKey] = true
	}
	var out []drift
	seen := map[drift]bool{}
	for _, t := range obs.Targets {
		if active[t.TargetKey] {
			continue
		}
		reason := "absent"
		if removed[t.TargetKey] {
			reason = "retained_removal"
		}
		d := drift{seed, t.TargetKey, t.RawCode, reason, expandHash, fetchHash}
		if !seen[d] {
			seen[d] = true
			out = append(out, d)
		}
	}
	return out
}

// Finalize rejects scope drift and freezes final membership in the close transaction.
func (*Stage) Finalize(ctx context.Context, env stage.Env, tx pgx.Tx, r stage.Run) (stage.Finalization, error) {
	list, err := source.Parse(r.Snapshot.SourceList)
	if err != nil {
		return stage.Finalization{}, fmt.Errorf("parse source list: %w", err)
	}
	if list.VBPL.SelectionVersion == 0 {
		return stage.Finalization{}, nil
	}
	if list.VBPL.SelectionVersion != 1 {
		return stage.Finalization{}, fmt.Errorf("unsupported selection version %d", list.VBPL.SelectionVersion)
	}
	staged := false
	for _, s := range r.Snapshot.Stages {
		if s == "expand" {
			staged = true
			break
		}
	}
	phase := membership.Seeds
	if staged {
		phase = membership.Expanded
	}
	q := store.New(tx)
	snapshot, err := membership.ReadPhase(ctx, q, r.ID, phase)
	if err != nil {
		return stage.Finalization{}, fmt.Errorf("read fetch membership: %w", err)
	}
	items, err := q.ListStageItemArtifacts(ctx, store.ListStageItemArtifactsParams{RunID: r.ID, Stage: "fetch"})
	if err != nil {
		return stage.Finalization{}, fmt.Errorf("list fetch items: %w", err)
	}
	type inputItem struct {
		Key    string  `json:"key"`
		State  string  `json:"state"`
		SHA256 *string `json:"artifact_sha256"`
	}
	inputs := make([]inputItem, 0, len(items))
	for _, item := range items {
		inputs = append(inputs, inputItem{item.ItemKey, item.State, item.Sha256})
	}
	data, err := json.Marshal(struct {
		ExpandedHash string      `json:"expanded_hash"`
		Items        []inputItem `json:"items"`
	}{snapshot.Hash, inputs})
	if err != nil {
		return stage.Finalization{}, fmt.Errorf("encode fetch close input: %w", err)
	}
	sum := sha256.Sum256(data)
	hash := hex.EncodeToString(sum[:])
	scope, err := q.GetRunScope(ctx, r.ID)
	if err != nil {
		return stage.Finalization{}, fmt.Errorf("read fetch scope: %w", err)
	}
	if scope.FetchCloseInputHash != nil && *scope.FetchCloseInputHash != hash {
		return stage.Finalization{}, fmt.Errorf("fetch close input changed after finalization")
	}
	if staged {
		expandItems, listErr := q.ListStageItemArtifacts(ctx, store.ListStageItemArtifactsParams{RunID: r.ID, Stage: "expand"})
		if listErr != nil {
			return stage.Finalization{}, fmt.Errorf("list expand items: %w", listErr)
		}
		hashes := map[string]string{}
		for _, item := range expandItems {
			if item.Sha256 != nil {
				hashes[item.ItemKey] = *item.Sha256
			}
		}
		seeds := map[string]bool{}
		for _, e := range snapshot.Active {
			if e.Source == "vbpl" && e.Class == "primary" && !strings.HasPrefix(strings.ToLower(e.SourceID), "vbpqta_") {
				seeds[e.DocumentKey] = true
			}
		}
		var drifts []drift
		for _, item := range items {
			if !seeds[item.ItemKey] || item.State != "done" || item.Path == nil || item.Sha256 == nil {
				continue
			}
			dir := env.Artifacts.Abs(*item.Path)
			actual, verifyErr := artifact.VerifyFiles(dir, "detail.json", "diagram.json")
			if verifyErr != nil {
				return stage.Finalization{}, fmt.Errorf("verify fetch artifact %s: %w", item.ItemKey, verifyErr)
			}
			if actual != *item.Sha256 {
				return stage.Finalization{}, fmt.Errorf("fetch artifact hash mismatch for %s", item.ItemKey)
			}
			detail, detailErr := os.ReadFile(filepath.Join(dir, "detail.json"))
			if detailErr != nil {
				return stage.Finalization{}, fmt.Errorf("read fetch detail: %w", detailErr)
			}
			diagram, diagramErr := os.ReadFile(filepath.Join(dir, "diagram.json"))
			if diagramErr != nil {
				return stage.Finalization{}, fmt.Errorf("read fetch diagram: %w", diagramErr)
			}
			obs, qualifyErr := expand.Qualify(item.ItemKey, detail, diagram, list.RelationCodes)
			if qualifyErr != nil {
				return stage.Finalization{}, fmt.Errorf("qualify fetch relations: %w", qualifyErr)
			}
			eh := hashes[item.ItemKey]
			if eh == "" {
				eh = "none"
			}
			drifts = append(drifts, findDrift(item.ItemKey, obs, snapshot, eh, *item.Sha256)...)
		}
		if len(drifts) > 0 {
			sort.Slice(drifts, func(i, j int) bool {
				a, b := drifts[i], drifts[j]
				if a.seed != b.seed {
					return a.seed < b.seed
				}
				if a.target != b.target {
					return a.target < b.target
				}
				if a.code != b.code {
					return a.code < b.code
				}
				return a.reason < b.reason
			})
			refs := make([]string, 0, len(drifts))
			for _, d := range drifts {
				refs = append(refs, fmt.Sprintf("seed=%s target=%s code=%s reason=%s expand=%s fetch=%s", d.seed, d.target, d.code, d.reason, d.expandHash, d.fetchHash))
			}
			sort.Strings(refs)
			first := drifts[0]
			return stage.Finalization{Code: "scope_changed", ItemKey: first.seed, Reason: fmt.Sprintf("%d scope drift(s): seed=%s target=%s code=%s reason=%s", len(drifts), first.seed, first.target, first.code, first.reason), EvidenceRefs: refs}, nil
		}
	}
	if err = membership.PublishPhase(ctx, q, r.ID, membership.Final, snapshot.Entries); err != nil {
		return stage.Finalization{}, fmt.Errorf("publish final membership: %w", err)
	}
	n, err := q.SetFetchCloseInputHash(ctx, store.SetFetchCloseInputHashParams{RunID: r.ID, FetchCloseInputHash: &hash})
	if err != nil {
		return stage.Finalization{}, fmt.Errorf("store fetch close input: %w", err)
	}
	if n != 1 {
		return stage.Finalization{}, fmt.Errorf("fetch close input changed after finalization")
	}
	return stage.Finalization{}, nil
}
