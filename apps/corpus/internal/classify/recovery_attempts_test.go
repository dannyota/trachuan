package classify

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRebuild_PreservesAbandonedAttempt(t *testing.T) {
	ctx := context.Background()
	r, ledger, client, o := screenFixture(t)
	client.beforeCreate = nil
	client.createErr = errors.New("synthetic lost response")
	if _, err := r.Screen(ctx, o, documents(), families()); err == nil {
		t.Fatal("expected uncertain submission")
	}
	abandoned := ledger.rows[0]
	r.Now = func() time.Time { return abandoned.CreatedAt.Add(25 * time.Hour) }
	if err := r.Reconcile(ctx, abandoned.ID, true); err != nil {
		t.Fatal(err)
	}
	client.createErr = nil
	client.ended = false
	if _, err := r.Screen(ctx, o, documents(), families()); err == nil {
		t.Fatal("expected interrupted polling")
	}
	ledger.rows = nil
	if err := r.Rebuild(ctx); err != nil {
		t.Fatal(err)
	}
	if len(ledger.rows) != 2 {
		t.Fatalf("restored %d attempts", len(ledger.rows))
	}
	for _, s := range ledger.rows {
		if s.ID == abandoned.ID && (s.State != "abandoned" || s.Settled || s.ReserveMicroUSD != 1029) {
			t.Fatalf("abandoned attempt %+v", s)
		}
	}
}

type trackingLedger struct {
	*memoryLedger
	locked bool
}

func (m *trackingLedger) Lock(context.Context) (func(), error) {
	if m.locked {
		return nil, errors.New("synthetic busy ledger")
	}
	m.locked = true
	return func() { m.locked = false }, nil
}

func TestScreen_ReleasesLockAndReloadsCancellationAfterWait(t *testing.T) {
	r, ledger, client, o := screenFixture(t)
	r.Repository = &trackingLedger{memoryLedger: ledger}
	client.ended = false
	r.Wait = func(ctx context.Context, _ time.Duration) error {
		unlock, err := r.Repository.Lock(ctx)
		if err != nil {
			return err
		}
		defer unlock()
		s := ledger.rows[0]
		s.State = "cancelling"
		if err = r.Repository.Save(ctx, s); err != nil {
			return err
		}
		client.ended = true
		return nil
	}
	if _, err := r.Screen(context.Background(), o, documents(), families()); err != nil {
		t.Fatal(err)
	}
	if ledger.rows[0].State != "cancelled" || !ledger.rows[0].Settled || ledger.rows[0].CostMicroUSD != 8 {
		t.Fatalf("final row %+v", ledger.rows[0])
	}
}

func TestScreen_BlocksAdmissionUntilIntentsAreReconstructed(t *testing.T) {
	r, ledger, client, o := screenFixture(t)
	client.ended = false
	if _, err := r.Screen(context.Background(), o, documents(), families()); err == nil {
		t.Fatal("expected interrupted polling")
	}
	ledger.rows = nil
	o.RunID++
	o.Release = "20990102"
	before := client.creates
	if _, err := r.Screen(context.Background(), o, documents(), families()); err == nil || client.creates != before {
		t.Fatal("submitted while the ledger omitted retained intents")
	}
}

type receiptRaceLedger struct {
	*memoryLedger
	beforeLock func()
}

func (m *receiptRaceLedger) Lock(ctx context.Context) (func(), error) {
	m.beforeLock()
	return m.memoryLedger.Lock(ctx)
}

func TestScreen_AttachmentPreservesConcurrentState(t *testing.T) {
	for _, state := range []string{"cancelling", "completed"} {
		t.Run(state, func(t *testing.T) {
			r, ledger, client, o := screenFixture(t)
			r.Repository = &receiptRaceLedger{memoryLedger: ledger, beforeLock: func() {
				if len(ledger.rows) != 1 || ledger.rows[0].BatchID != "" {
					return
				}
				s := ledger.rows[0]
				known, err := r.restoreReceipt(&s)
				if err != nil {
					t.Fatal(err)
				}
				if !known {
					return
				}
				if state == "completed" {
					if err = apply(&s, client.results); err != nil {
						t.Fatal(err)
					}
				} else {
					s.State = "cancelling"
				}
				ledger.rows[0] = s
			}}
			if _, err := r.Screen(context.Background(), o, documents(), families()); err != nil {
				t.Fatal(err)
			}
			want := "cancelled"
			if state == "completed" {
				want = "completed"
			}
			if ledger.rows[0].State != want || !ledger.rows[0].Settled || ledger.rows[0].CostMicroUSD != 8 {
				t.Fatalf("state after attach %+v", ledger.rows[0])
			}
			if state == "completed" && client.downloads != 0 {
				t.Fatal("attachment reopened an already settled submission")
			}
		})
	}
}

func TestCancel_RepeatedCancellationNeedsNoProvider(t *testing.T) {
	r, ledger, client, o := screenFixture(t)
	client.ended = false
	if _, err := r.Screen(context.Background(), o, documents(), families()); err == nil {
		t.Fatal("expected interrupted polling")
	}
	client.ended = true
	if err := r.Cancel(context.Background(), ledger.rows[0].ID, o.Out); err != nil {
		t.Fatal(err)
	}
	r.Client = nil
	if err := r.Cancel(context.Background(), ledger.rows[0].ID, o.Out); err != nil {
		t.Fatal("repeated cancellation required a provider call", err)
	}
}
