//go:build integration

package classify

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/batchapi"
	"github.com/anthropics/anthropic-sdk-go/v2"
)

func TestScreen_DatabasePausedCreatorCannotSubmitAfterReplacement(t *testing.T) {
	for _, release := range []int32{1, 2} {
		t.Run(fmt.Sprint(release), func(t *testing.T) {
			db, id, rel := submissionDatabase(t)
			r, _, client, o := screenFixture(t)
			o.RunID, o.Release = id, rel
			creatorInterleaving(t, r, client, o, func() Repository { return &PGRepository{Pool: db} }, release)
		})
	}
}

type stalledReconcileClient struct {
	*pollingClient
	phase   string
	entered chan struct{}
	resume  chan struct{}
}

func (c *stalledReconcileClient) stall(ctx context.Context) error {
	close(c.entered)
	select {
	case <-c.resume:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *stalledReconcileClient) List(ctx context.Context, _, _ time.Time) ([]anthropic.MessageBatch, error) {
	if c.phase == "list" {
		if err := c.stall(ctx); err != nil {
			return nil, err
		}
	}
	return []anthropic.MessageBatch{{ID: "unrelated_synthetic", ProcessingStatus: "ended"}}, nil
}

func (c *stalledReconcileClient) Results(ctx context.Context, id string) ([]batchapi.Result, error) {
	if id == "unrelated_synthetic" {
		if c.phase == "results" {
			if err := c.stall(ctx); err != nil {
				return nil, err
			}
		}
		return nil, nil
	}
	return c.pollingClient.Results(ctx, id)
}

func TestReconcile_DatabaseCancellationDuringProviderCalls(t *testing.T) {
	for _, phase := range []string{"list", "results"} {
		t.Run(phase, func(t *testing.T) {
			db, id, rel := submissionDatabase(t)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			r, _, _, o := screenFixture(t)
			o.RunID, o.Release = id, rel
			r.Repository = &PGRepository{Pool: db}
			client := &pollingClient{}
			r.Client = client
			if err := client.message.UnmarshalJSON([]byte(`{"stop_reason":"end_turn","content":[{"type":"text","text":"{\"codes\":[\"1.1\"]}"}],"usage":{"input_tokens":100,"output_tokens":10}}`)); err != nil {
				t.Fatal(err)
			}
			if _, err := r.Screen(ctx, o, documents(), families()); err == nil {
				t.Fatal("expected interrupted polling")
			}
			var submittedID string
			if err := db.QueryRow(ctx, `SELECT id FROM submission WHERE run_id=$1`, id).Scan(&submittedID); err != nil {
				t.Fatal(err)
			}
			stalled := Submission{ID: strings.Repeat("f", 32), RunID: id, Country: o.Country, Release: rel, ScreenHash: strings.Repeat("b", 64), Recipe: o.Recipe, Families: families(), Items: []Item{{Key: "vn:vbpl:1", InputTokens: 100}}, State: "uncertain", CreatedAt: r.Now(), Budget: o.Budget, BudgetMicroUSD: o.BudgetMicroUSD, ReserveMicroUSD: 1029}
			stalled.ItemsHash = itemsHash(stalled.Items)
			unlock, err := r.Repository.Lock(ctx)
			if err != nil {
				t.Fatal(err)
			}
			err = r.Repository.Save(ctx, stalled)
			unlock()
			if err != nil {
				t.Fatal(err)
			}
			if err = r.writeIntent(stalled); err != nil {
				t.Fatal(err)
			}
			blocking := &stalledReconcileClient{pollingClient: client, phase: phase, entered: make(chan struct{}), resume: make(chan struct{})}
			reconciler := &Runner{Repository: &PGRepository{Pool: db}, Client: blocking, IntentDir: r.IntentDir, Now: r.Now}
			reconciled := make(chan error, 1)
			go func() { reconciled <- reconciler.Reconcile(ctx, stalled.ID, false) }()
			select {
			case <-blocking.entered:
			case <-ctx.Done():
				t.Fatal("reconciliation never reached provider call")
			}
			observer := &PGRepository{Pool: db}
			listCtx, stopListing := context.WithTimeout(ctx, 100*time.Millisecond)
			unlock, err = observer.Lock(listCtx)
			if err == nil {
				_, err = observer.List(listCtx)
				unlock()
			}
			stopListing()
			if err != nil {
				t.Fatal("stalled reconciliation blocked listing", err)
			}
			client.mu.Lock()
			client.ended = true
			client.mu.Unlock()
			stopper := &Runner{Repository: &PGRepository{Pool: db}, Client: client, IntentDir: r.IntentDir, Now: r.Now, Wait: func(context.Context, time.Duration) error { return errors.New("unexpected wait") }}
			if err = stopper.Cancel(ctx, submittedID, filepath.Clean(o.Out)); err != nil {
				t.Fatal("stalled reconciliation blocked cancellation", err)
			}
			var state string
			var cost int64
			var settled bool
			if err = db.QueryRow(ctx, `SELECT state,cost_micro_usd,cost_settled FROM submission WHERE id=$1`, submittedID).Scan(&state, &cost, &settled); err != nil {
				t.Fatal(err)
			}
			if state != "cancelled" || cost != 8 || !settled {
				t.Fatalf("state %s cost %d settled %t", state, cost, settled)
			}
			close(blocking.resume)
			select {
			case err = <-reconciled:
				if err == nil {
					t.Fatal("unexpected match for unrelated batch")
				}
			case <-ctx.Done():
				t.Fatal("reconciliation did not stop")
			}
		})
	}
}
