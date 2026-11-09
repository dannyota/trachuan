package run

import (
	"context"
	"fmt"

	"github.com/dannyota/traquydinh/apps/corpus/internal/store"
)

// Recover fences old activities and optionally terminates an open workflow.
func Recover(ctx context.Context, d Deps, cc, rel string, terminate bool) error {
	tx, err := d.DB.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin recovery: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := store.New(tx)
	row, err := q.GetRunForUpdate(ctx, store.GetRunForUpdateParams{Country: cc, Rel: rel})
	if err != nil {
		return fmt.Errorf("lock run: %w", err)
	}
	if err = q.RecoverRun(ctx, row.ID); err != nil {
		return fmt.Errorf("recover run: %w", err)
	}
	if terminate {
		id := WorkflowID(cc, rel)
		open, openErr := workflowOpen(ctx, d, id)
		if openErr != nil {
			return openErr
		}
		if open {
			if terminateErr := d.Temporal.TerminateWorkflow(ctx, id, "", "recovered"); terminateErr != nil {
				return fmt.Errorf("terminate workflow: %w", terminateErr)
			}
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit recovery: %w", err)
	}
	return nil
}
