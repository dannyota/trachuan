package discover

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"danny.vn/trachuan/apps/corpus/internal/stage/membership"
	"danny.vn/trachuan/apps/corpus/internal/store"
	"github.com/jackc/pgx/v5"
)

func verifyFrozenEvidence(ctx context.Context, q *store.Queries, runID int64, members map[string]*seedMember) (bool, error) {
	scope, err := q.GetRunScope(ctx, runID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read seed scope: %w", err)
	}
	if scope.SeedsSnapshot == nil {
		return false, nil
	}
	existing, err := membership.ReadEvidence(ctx, q, runID)
	if err != nil {
		return false, fmt.Errorf("read frozen evidence: %w", err)
	}
	counts := map[string]int{}
	key := func(r membership.EvidenceRecord) (string, error) {
		b, e := json.Marshal(r)
		if e != nil {
			return "", fmt.Errorf("encode evidence: %w", e)
		}
		return string(b), nil
	}
	for _, record := range existing {
		if record.Stage != "discover" || (record.Support.RuleKind != "agency" && record.Support.RuleKind != "term" && record.Support.RuleKind != "named") {
			continue
		}
		k, e := key(record)
		if e != nil {
			return false, e
		}
		counts[k]++
	}
	for _, member := range members {
		for _, record := range member.records {
			k, e := key(record)
			if e != nil {
				return false, e
			}
			counts[k]--
		}
	}
	for _, n := range counts {
		if n != 0 {
			return false, fmt.Errorf("discover evidence changed after seeds were frozen")
		}
	}
	return true, nil
}
