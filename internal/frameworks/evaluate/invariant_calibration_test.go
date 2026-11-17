package evaluate

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
)

type calibrationLeakWorkload struct{ workloadEngine }

func (e calibrationLeakWorkload) CalibrationSearch(ctx context.Context, query string) (*float64, bool, error) {
	score, lookup, err := e.workloadEngine.CalibrationSearch(ctx, query)
	controlChanged := e.state.changed("fw-alpha", "control_text") || e.state.changed("fw-gamma", "control_text")
	bodyChanged := e.state.changed("fw-alpha", "body_index") || e.state.changed("fw-gamma", "body_index")
	if controlChanged && !bodyChanged && strings.HasPrefix(query, "fixture-negative") {
		value := .4
		score = &value
	}
	return score, lookup, err
}
func TestInvariant_CalibratesBaseAndEveryStage(t *testing.T) {
	for _, test := range []struct {
		name string
		leak bool
	}{{name: "base threshold mismatch"}, {name: "early stage mismatch", leak: true}} {
		t.Run(test.name, func(t *testing.T) {
			req := workloadRequest()
			counts := &workloadCounts{}
			copies := workloadCopies(counts, NewQueryCache(workloadQuery{counts}), req.Cases)
			if test.leak {
				original := copies.open
				copies.open = func(ctx context.Context, conn *pgx.Conn, key []byte) (Engine, func(), error) {
					engine, closeEngine, err := original(ctx, conn, key)
					if err != nil {
						return nil, nil, err
					}
					return calibrationLeakWorkload{workloadEngine: engine.(workloadEngine)}, closeEngine, nil
				}
			} else {
				req.Calibration.Threshold = .6
			}
			result := workloadInvariant(t, req, copies)
			require.False(t, result.Pass)
			require.Equal(t, 864, counts.calibrations)
			for _, caller := range result.Callers {
				require.Equal(t, "fail", caller.Calibration)
				if test.leak {
					require.Len(t, caller.Failures, 1)
				} else {
					require.Len(t, caller.Failures, 6)
				}
				for _, failure := range caller.Failures {
					require.Equal(t, "recalibration", failure.Operation)
					require.NotEmpty(t, failure.Cause.GoType)
					if test.leak {
						require.Equal(t, "control_text", failure.Class)
					}
				}
				for _, class := range privateClasses() {
					require.True(t, caller.Classes[class].PositiveControl)
					require.Equal(t, test.leak && class != "control_text", caller.Classes[class].Pass)
				}
			}
		})
	}
}
