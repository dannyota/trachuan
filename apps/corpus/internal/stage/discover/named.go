package discover

import (
	"context"
	"fmt"

	"danny.vn/trachuan/apps/corpus/internal/lookup"
)

func (s *Stage) named(ctx context.Context, dir, number string) error {
	report := scopeReport{Version: 2, Passes: []lookup.Result{}}
	var previous []string
	var final lookup.Result
	for pass := 1; pass <= 3; pass++ {
		result, bodies, err := s.client.LookupNumber(ctx, number)
		if err != nil {
			return retryLookup(err)
		}
		for i, body := range bodies {
			ref, writeErr := writePage(dir, pass, i+1, body)
			if writeErr != nil {
				return writeErr
			}
			result.Evidence.PagesFetched[i].Path = ref.Path
		}
		report.QueryIdentity = result.Evidence.QueryIdentity
		report.Passes = append(report.Passes, result)
		final = result
		current := ids(result.Items)
		if len(report.Passes) > 1 && result.Evidence.Completeness == lookup.Complete && report.Passes[len(report.Passes)-2].Evidence.Completeness == lookup.Complete && sameIDs(previous, current) {
			report.Stable = true
			break
		}
		previous = current
	}
	if err := writeJSON(dir, "report.json", report); err != nil {
		return err
	}
	if err := writeJSON(dir, "lookup.json", final); err != nil {
		return fmt.Errorf("write lookup result: %w", err)
	}
	return nil
}
