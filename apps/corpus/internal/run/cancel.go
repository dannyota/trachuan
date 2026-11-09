package run

import (
	"context"
	"fmt"

	"github.com/dannyota/traquydinh/apps/corpus/internal/store"
)

// Cancel requests cancellation of an open workflow and records its state.
func Cancel(ctx context.Context, d Deps, cc, rel string) error {
	tx, err := d.DB.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin cancellation: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := store.New(tx)
	row, err := q.GetRunForUpdate(ctx, store.GetRunForUpdateParams{Country: cc, Rel: rel})
	if err != nil {
		return fmt.Errorf("lock run: %w", err)
	}
	if row.State == "evaluated" || row.State == "released" {
		return fmt.Errorf("cannot modify %s run", row.State)
	}
	open, err := workflowOpen(ctx, d, WorkflowID(cc, rel))
	if err != nil {
		return err
	}
	state := "cancelled"
	if open && (row.State == "starting" || row.State == "running" || row.State == "cancelling") {
		state = "cancelling"
	}
	if err = q.SetRunState(ctx, store.SetRunStateParams{ID: row.ID, State: state}); err != nil {
		return fmt.Errorf("mark cancellation: %w", err)
	}
	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit cancellation: %w", err)
	}
	return nil
}
