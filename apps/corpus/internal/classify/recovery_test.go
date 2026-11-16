package classify

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/batchapi"
	"github.com/anthropics/anthropic-sdk-go/v2"
)

func (f *fakeClient) List(context.Context, time.Time, time.Time) ([]anthropic.MessageBatch, error) {
	return []anthropic.MessageBatch{{ID: "batch_synthetic", ProcessingStatus: "ended", CreatedAt: f.batchCreated}}, nil
}
func (f *fakeClient) Cancel(context.Context, string) error { return nil }

func TestRebuild_RestoresUncertainOrKnownBatch(t *testing.T) {
	for _, known := range []bool{false, true} {
		t.Run(map[bool]string{false: "uncertain", true: "submitted"}[known], func(t *testing.T) {
			r, m, _, o := screenFixture(t)
			s := Submission{ID: strings.Repeat("a", 32), RunID: o.RunID, Country: o.Country, Release: o.Release, Recipe: o.Recipe, Items: []Item{{Key: "vn:vbpl:1", InputTokens: 100}}, State: "preparing", CreatedAt: r.Now(), ReserveMicroUSD: 1029, Budget: "pilot", BudgetMicroUSD: 15000000}
			s.ItemsHash = itemsHash(s.Items)
			if err := r.writeIntent(s); err != nil {
				t.Fatal(err)
			}
			if known {
				if err := r.attach(context.Background(), &s, "batch_synthetic"); err != nil {
					t.Fatal(err)
				}
			}
			m.rows = nil
			if err := r.Rebuild(context.Background()); err != nil {
				t.Fatal(err)
			}
			if len(m.rows) != 1 || m.rows[0].ReserveMicroUSD != 1029 || m.rows[0].Settled {
				t.Fatalf("rows %+v", m.rows)
			}
			want := "uncertain"
			if known {
				want = "submitted"
			}
			if m.rows[0].State != want {
				t.Fatalf("state %s", m.rows[0].State)
			}
			if err := r.Rebuild(context.Background()); err != nil || len(m.rows) != 1 {
				t.Fatal("rebuild duplicated ledger", err)
			}
		})
	}
}

func TestReconcile_AttachesOnlyMatchingPrefixAndWaitsBeforeAbandon(t *testing.T) {
	r, m, f, o := screenFixture(t)
	f.createErr = context.DeadlineExceeded
	if _, err := r.Screen(context.Background(), o, documents(), families()); err == nil {
		t.Fatal("expected lost response")
	}
	id := m.rows[0].ID
	f.results = []batchapi.Result{{CustomID: strings.Repeat("b", 32) + "-000000", Type: "expired"}}
	if err := r.Reconcile(context.Background(), id, true); err == nil || m.rows[0].State != "uncertain" {
		t.Fatal("abandoned before 24 hours")
	}
	f.results = []batchapi.Result{{CustomID: id + "-000000", Type: "expired"}}
	if err := r.Reconcile(context.Background(), id, false); err != nil || m.rows[0].State != "submitted" {
		t.Fatal("failed to reconcile", err)
	}
}

func TestRebuild_RejectsTamperedItemOrder(t *testing.T) {
	r, _, _, o := screenFixture(t)
	s := Submission{ID: strings.Repeat("a", 32), RunID: 1, Recipe: o.Recipe, Items: []Item{{Key: "vn:vbpl:1"}}, ItemsHash: strings.Repeat("0", 64), State: "preparing"}
	b, _ := json.Marshal(s)
	path := filepath.Join(r.IntentDir, "submissions")
	if err := os.MkdirAll(path, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, s.ID+".json"), b, 0600); err != nil {
		t.Fatal(err)
	}
	if err := r.Rebuild(context.Background()); err == nil {
		t.Fatal("accepted changed ordered items")
	}
}

func TestReconcile_DoesNotRestartDeadline(t *testing.T) {
	r, m, f, o := screenFixture(t)
	f.createErr = context.DeadlineExceeded
	if _, err := r.Screen(context.Background(), o, documents(), families()); err == nil {
		t.Fatal("expected lost response")
	}
	created := m.rows[0].CreatedAt
	r.Now = func() time.Time { return created.Add(25 * time.Hour) }
	if err := r.Reconcile(context.Background(), m.rows[0].ID, false); err != nil {
		t.Fatal(err)
	}
	if !m.rows[0].DeadlineAt.Equal(created.Add(26 * time.Hour)) {
		t.Fatal("recovery extended polling deadline")
	}
}

func TestRebuild_RejectsReservationMismatch(t *testing.T) {
	r, m, _, o := screenFixture(t)
	s := Submission{ID: strings.Repeat("a", 32), RunID: 1, Recipe: o.Recipe, Items: []Item{{Key: "vn:vbpl:1", InputTokens: 100}}, State: "preparing", CreatedAt: r.Now(), ReserveMicroUSD: 1}
	s.ItemsHash = itemsHash(s.Items)
	if err := r.writeIntent(s); err != nil {
		t.Fatal(err)
	}
	if err := r.Rebuild(context.Background()); err == nil || len(m.rows) != 0 {
		t.Fatal("accepted understated recovered reservation")
	}
}
