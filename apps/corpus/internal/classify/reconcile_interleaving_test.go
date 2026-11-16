package classify

import (
	"context"
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/batchapi"
	"github.com/anthropics/anthropic-sdk-go/v2"
)

type hookedReconcileClient struct {
	*fakeClient
	listHook    func() error
	resultsHook func() error
}

func (c *hookedReconcileClient) List(ctx context.Context, after, before time.Time) ([]anthropic.MessageBatch, error) {
	if c.listHook != nil {
		if err := c.listHook(); err != nil {
			return nil, err
		}
	}
	return c.fakeClient.List(ctx, after, before)
}

func (c *hookedReconcileClient) Results(ctx context.Context, id string) ([]batchapi.Result, error) {
	if c.resultsHook != nil {
		if err := c.resultsHook(); err != nil {
			return nil, err
		}
	}
	return c.fakeClient.Results(ctx, id)
}

func TestReconcile_ProviderCallsReleaseLedger(t *testing.T) {
	for _, phase := range []string{"list", "results"} {
		t.Run(phase, func(t *testing.T) {
			r, ledger, client, o := screenFixture(t)
			r.Repository = &trackingLedger{memoryLedger: ledger}
			client.createErr = context.DeadlineExceeded
			if _, err := r.Screen(context.Background(), o, documents(), families()); err == nil {
				t.Fatal("expected uncertain create response")
			}
			hook := func() error {
				unlock, err := r.Repository.Lock(context.Background())
				if err == nil {
					unlock()
				}
				return err
			}
			wrapped := &hookedReconcileClient{fakeClient: client}
			if phase == "list" {
				wrapped.listHook = hook
			} else {
				wrapped.resultsHook = hook
			}
			r.Client = wrapped
			if err := r.Reconcile(context.Background(), ledger.rows[0].ID, false); err != nil {
				t.Fatal("provider call blocked another ledger operation", err)
			}
		})
	}
}

func TestReconcile_ReloadsTargetAfterProviderCalls(t *testing.T) {
	r, ledger, client, o := screenFixture(t)
	client.createErr = context.DeadlineExceeded
	if _, err := r.Screen(context.Background(), o, documents(), families()); err == nil {
		t.Fatal("expected uncertain create response")
	}
	r.Client = &hookedReconcileClient{fakeClient: client, listHook: func() error {
		ledger.rows[0].State = "failed"
		return nil
	}}
	if err := r.Reconcile(context.Background(), ledger.rows[0].ID, false); err == nil {
		t.Fatal("accepted a target changed during reconciliation")
	}
	if ledger.rows[0].State != "failed" || ledger.rows[0].BatchID != "" {
		t.Fatal("reconciliation overwrote newer state")
	}
}

func TestReconcile_SettlesLateBatchOnAbandonedAttempt(t *testing.T) {
	r, ledger, client, o := screenFixture(t)
	client.createErr = context.DeadlineExceeded
	if _, err := r.Screen(context.Background(), o, documents(), families()); err == nil {
		t.Fatal("expected uncertain create response")
	}
	id := ledger.rows[0].ID
	late := client.results
	client.results = nil
	created := r.Now()
	r.Now = func() time.Time { return created.Add(25 * time.Hour) }
	if err := r.Reconcile(context.Background(), id, true); err != nil {
		t.Fatal(err)
	}
	client.results = late
	if err := r.Reconcile(context.Background(), id, false); err != nil {
		t.Fatal("late provider batch could not be billed", err)
	}
	s := ledger.rows[0]
	if s.State != "abandoned" || !s.Settled || s.CostMicroUSD != 8 || s.BatchID != "batch_synthetic" {
		t.Fatalf("late batch ledger %+v", s)
	}
}

func TestReconcile_UnknownCreatorRequiresObservedQuietWindow(t *testing.T) {
	r, ledger, client, o := screenFixture(t)
	s := Submission{ID: strings.Repeat("a", 32), RunID: o.RunID, Country: o.Country, Release: o.Release, ScreenHash: screenHash(o), Recipe: o.Recipe, Items: []Item{{Key: "vn:vbpl:1", InputTokens: 100}}, State: "uncertain", CreatedAt: r.Now(), ReserveMicroUSD: 1029, Budget: o.Budget, BudgetMicroUSD: o.BudgetMicroUSD}
	s.ItemsHash = itemsHash(s.Items)
	ledger.rows = []Submission{s}
	if err := r.writeIntent(s); err != nil {
		t.Fatal(err)
	}
	client.results = nil
	observed := s.CreatedAt.Add(25 * time.Hour)
	r.Now = func() time.Time { return observed }
	if err := r.Reconcile(context.Background(), s.ID, true); err == nil {
		t.Fatal("abandoned a creator with unknown stop time without a quiet window")
	}
	r.Now = func() time.Time { return observed.Add(24*time.Hour + 10*time.Minute) }
	if err := r.Reconcile(context.Background(), s.ID, true); err != nil {
		t.Fatal("quiet window did not permit abandonment", err)
	}
}
