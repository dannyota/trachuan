package evidence

import (
	"context"
	"slices"
	"strings"

	"danny.vn/trachuan/apps/corpus/internal/detector"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/store"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Count struct {
	Detector      string
	Severity      detector.Severity
	Findings      int
	Classes       map[string]int
	Observed      int
	NotObservable int
	NotRun        string
}

func CountInputs(ctx context.Context, q store.Querier, env stage.Env, run stage.Run, release *pgxpool.Pool) Inputs {
	return Inputs{Run: run, Release: release, ArtifactDir: lazyArtifactDirs(ctx, func() store.Querier { return q }, env, run)}
}

func CountFindings(ctx context.Context, in Inputs, detectors []Detector) []Count {
	counts := make([]Count, 0, len(detectors))
	for _, d := range detectors {
		spec := d.Spec()
		count := Count{Detector: spec.ID, Severity: spec.Severity}
		obs, err := d.Detect(ctx, in)
		if err != nil {
			count.NotRun = err.Error()
			counts = append(counts, count)
			continue
		}
		count.Findings, count.Observed, count.NotObservable = len(obs.Findings), obs.Observed, obs.NotObservable
		for _, f := range obs.Findings {
			if f.Severity != "" {
				if count.Classes == nil {
					count.Classes = map[string]int{}
				}
				count.Classes[string(f.Severity)]++
			}
		}
		counts = append(counts, count)
	}
	slices.SortFunc(counts, func(a, b Count) int { return strings.Compare(a.Detector, b.Detector) })
	return counts
}
