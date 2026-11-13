package stage

import (
	"context"
	"fmt"

	"danny.vn/trachuan/apps/corpus/internal/store"
	"github.com/jackc/pgx/v5"
)

type preflighter interface {
	Preflight(context.Context, Env, pgx.Tx, Run) (Finalization, error)
}

func preflight(ctx context.Context, env Env, tx pgx.Tx, st Stage, run Run) (bool, error) {
	p, ok := st.(preflighter)
	if !ok {
		return false, nil
	}
	q := store.New(tx)
	failures, err := q.ListRunFailures(ctx, run.ID)
	if err != nil {
		return false, fmt.Errorf("read preflight failures: %w", err)
	}
	for _, failure := range failures {
		for _, code := range BlockingFailureCodes() {
			if failure.Code == code {
				return true, nil
			}
		}
	}
	failure, err := p.Preflight(ctx, env, tx, run)
	if err != nil {
		return false, fmt.Errorf("stage preflight: %w", err)
	}
	if failure.Code == "" {
		return false, nil
	}
	if err = recordFinalization(ctx, q, StageInput{RunID: run.ID, Generation: run.Generation, Stage: st.Name()}, failure); err != nil {
		return false, err
	}
	return true, nil
}
