package expand

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"

	"danny.vn/trachuan/apps/corpus/internal/stage/membership"
	"danny.vn/trachuan/apps/corpus/internal/store"
)

func verifyFrozen(ctx context.Context, q *store.Queries, id int64, observations []store.UpsertSeedObservationParams, records []membership.EvidenceRecord) (bool, error) {
	scope, err := q.GetRunScope(ctx, id)
	if err != nil {
		return false, fmt.Errorf("read expanded scope: %w", err)
	}
	if scope.ExpandedSnapshot == nil {
		return false, nil
	}
	old, err := q.ListSeedObservations(ctx, id)
	if err != nil {
		return false, fmt.Errorf("read frozen observations: %w", err)
	}
	if len(old) != len(observations) {
		return false, fmt.Errorf("expand observations changed after freeze")
	}
	for i, o := range observations {
		var a, b []string
		if err = json.Unmarshal(old[i].ObservedTargets, &a); err != nil {
			return false, fmt.Errorf("decode frozen targets: %w", err)
		}
		if err = json.Unmarshal(o.ObservedTargets, &b); err != nil {
			return false, fmt.Errorf("decode targets: %w", err)
		}
		if old[i].SeedKey != o.SeedKey || old[i].ItemKey != o.ItemKey || old[i].Completeness != o.Completeness || !reflect.DeepEqual(old[i].ArtifactHash, o.ArtifactHash) || !reflect.DeepEqual(a, b) {
			return false, fmt.Errorf("expand observations changed after freeze")
		}
	}
	existing, err := membership.ReadEvidence(ctx, q, id)
	if err != nil {
		return false, fmt.Errorf("read frozen evidence: %w", err)
	}
	want := map[string]bool{}
	got := map[string]bool{}
	for _, rec := range records {
		b, e := json.Marshal(rec)
		if e != nil {
			return false, fmt.Errorf("encode evidence: %w", e)
		}
		want[string(b)] = true
	}
	for _, rec := range existing {
		if rec.Support.RuleKind != "relation" {
			continue
		}
		b, e := json.Marshal(rec)
		if e != nil {
			return false, fmt.Errorf("encode evidence: %w", e)
		}
		got[string(b)] = true
	}
	if !reflect.DeepEqual(want, got) {
		return false, fmt.Errorf("expand evidence changed after freeze")
	}
	return true, nil
}
