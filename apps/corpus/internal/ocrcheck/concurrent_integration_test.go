//go:build integration

package ocrcheck

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/ocrstore"
	"danny.vn/trachuan/apps/corpus/internal/store"
	"github.com/jackc/pgx/v5/pgtype"
)

func databaseJPEGFixture(t *testing.T, n int) (*Runner, *concurrentClient, []Document) {
	t.Helper()
	r, db, old, docs := databaseFixture(t)
	r.Now = time.Now
	_, _, _, _, files := jpegFixture(t)
	r.JPEGRenderer = concurrentRenderer{paths: files.paths}
	c := &concurrentClient{Client: old, tokens: 100, response: old.Response}
	r.Client = c
	source := ocrstore.Open(r.Repository.(*Database).root, db)
	for i := 1; i < n; i++ {
		engine := fmt.Sprintf("test-engine-%d", i)
		if _, err := source.Put(context.Background(), docs[0].Entry.PdfSha256, engine, "vision", pgtype.Int4{}, docs[0].Text); err != nil {
			t.Fatal(err)
		}
		entry, err := store.New(db).GetOCRText(context.Background(), store.GetOCRTextParams{PdfSha256: docs[0].Entry.PdfSha256, EngineID: engine})
		if err != nil {
			t.Fatal(err)
		}
		d := docs[0]
		d.Entry = entry
		docs = append(docs, d)
	}
	return r, c, docs
}

func TestConcurrent_DatabaseAtomicBudget(t *testing.T) {
	r, c, docs := databaseJPEGFixture(t, 12)
	c.response.Usage = Usage{Input: 100, Output: 32000}
	o := jpegOptions()
	o.MaxUSD = .960600
	s, err := r.Run(context.Background(), docs, o)
	if !errors.Is(err, ErrBudget) || c.calls != 3 || s.Complete != 3 {
		t.Fatalf("calls=%d summary=%+v err=%v", c.calls, s, err)
	}
	spent, err := r.Repository.Spent(context.Background(), o.CheckerID())
	if err != nil || spent != 960600 {
		t.Fatalf("spent=%d err=%v", spent, err)
	}
}

func TestConcurrent_DatabaseRealCostAndVersions(t *testing.T) {
	r, c, docs := databaseJPEGFixture(t, 2)
	o := jpegOptions()
	o.LimitDocs = 1
	o.MaxUSD = .320200
	db := r.Repository.(*Database)
	c.call = func(ctx context.Context, _ Request) (Response, error) {
		observer := NewDatabase(db.pool, db.root)
		open, err := observer.Open(ctx, o.CheckerID())
		if err != nil || len(open) != 1 || open[0].State != "preparing" || open[0].Usage.charge() != 320200 {
			return Response{}, fmt.Errorf("durable intent missing")
		}
		return c.response, nil
	}
	s, err := r.Run(context.Background(), docs, o)
	if err != nil || s.Complete != 1 || s.CostMicroUSD != 1200 {
		t.Fatalf("summary=%+v err=%v", s, err)
	}
	r.Repository = NewDatabase(db.pool, db.root)
	o.MaxUSD = .321400
	s, err = r.Run(context.Background(), docs, o)
	if err != nil || s.Complete != 1 || c.calls != 2 || s.CostMicroUSD != 2400 {
		t.Fatalf("summary=%+v calls=%d err=%v", s, c.calls, err)
	}
	complete, err := r.Repository.Completed(context.Background(), docs[0].Entry.ID, options().CheckerID())
	if err != nil || complete {
		t.Fatal("version 3 satisfied version 2")
	}
}

type crashRepository struct{ Repository }

func (crashRepository) Finish(context.Context, Attempt, store.OcrText, Usage, *ocrstore.CheckResult, string) error {
	return errors.New("simulated crash")
}

func TestConcurrent_DatabaseCrashRecovery(t *testing.T) {
	r, c, docs := databaseJPEGFixture(t, 8)
	db := r.Repository.(*Database)
	r.Repository = crashRepository{Repository: db}
	o := jpegOptions()
	if _, err := r.Run(context.Background(), docs, o); err == nil {
		t.Fatal("expected crash")
	}
	calls := c.calls
	r.Repository = NewDatabase(db.pool, db.root)
	s, err := r.Run(context.Background(), docs, o)
	if err != nil || c.calls != calls || s.Pending != calls || s.CostMicroUSD != int64(calls)*320200 {
		t.Fatalf("summary=%+v err=%v", s, err)
	}
	open, err := r.Repository.Open(context.Background(), o.CheckerID())
	if err != nil || len(open) != calls {
		t.Fatalf("open=%+v err=%v", open, err)
	}
	for _, a := range open {
		if a.State != "preparing" {
			t.Fatal(a.State)
		}
	}
	r.Now = func() time.Time { return open[0].CreatedAt.Add(11 * time.Minute) }
	if _, err = r.Run(context.Background(), docs, o); err == nil || c.calls != calls {
		t.Fatal("stale intent resent")
	}
	o.Reconcile = true
	if _, err = r.Run(context.Background(), nil, o); err == nil {
		t.Fatal("direct attempt reconciled as batch")
	}
	o.Reconcile = false
	o.Abandon = open[0].SubmissionID
	if _, err = r.Run(context.Background(), nil, o); err == nil {
		t.Fatal("early abandonment allowed")
	}
	r.Now = func() time.Time { return open[0].CreatedAt.Add(24 * time.Hour) }
	if _, err = r.Run(context.Background(), nil, o); err != nil {
		t.Fatal(err)
	}
	spent, err := r.Repository.Spent(context.Background(), o.CheckerID())
	if err != nil || spent != int64(calls)*320200 {
		t.Fatalf("lost hold: %d %v", spent, err)
	}
}
