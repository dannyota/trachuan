package ocrcheck

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestCheck_StaleBatchIntentMarkedOnce(t *testing.T) {
	r, s, c, d := fixture(t)
	o := options()
	o.Batch = true
	next := d[0]
	next.Entry.ID = 2
	d = append(d, next)
	// Simulate a crash after atomic intent, before the provider was called.
	if _, err := r.begin(context.Background(), []prepared{{doc: d[0], worst: 160100, inputTokens: 100}, {doc: d[1], worst: 160100, inputTokens: 100}}, o); err != nil {
		t.Fatal(err)
	}
	r.Now = func() time.Time { return s.attempts[0].CreatedAt.Add(11 * time.Minute) }
	if err := run(t, r, d, o); err == nil || !strings.Contains(err.Error(), "blocks new work") {
		t.Fatalf("stale intent error=%v", err)
	}
	for _, a := range s.attempts {
		if a.State != "uncertain" {
			t.Fatal("not all stale items marked uncertain")
		}
	}
	if c.BatchCalls != 0 {
		t.Fatal("resubmitted uncertain batch")
	}
	// Reconciliation must still work after all members were marked in one update.
	o.Reconcile = true
	r.Now = func() time.Time { return s.attempts[0].CreatedAt.Add(25 * time.Hour) }
	if err := run(t, r, d, o); err != nil {
		t.Fatal(err)
	}
	for _, a := range s.attempts {
		if a.State != "uncertain" {
			t.Fatal("reconciliation released a stale batch")
		}
	}
}
func TestCheck_BatchOutcomesByCustomID(t *testing.T) {
	for _, kind := range []string{"succeeded", "expired", "canceled", "errored", "unknown"} {
		t.Run(kind, func(t *testing.T) {
			r, s, c, d := fixture(t)
			o := options()
			o.Batch = true
			second := d[0]
			second.Entry.ID = 2
			d = append(d, second)
			if err := run(t, r, d, o); err != nil {
				t.Fatal(err)
			}
			c.BatchEnded = true
			c.Results = []BatchResult{{CustomID: customID(s.attempts[1]), Type: kind, Response: c.Response}, {CustomID: customID(s.attempts[0]), Type: "succeeded", Response: c.Response}}
			if err := run(t, r, d, o); err != nil {
				t.Fatal(err)
			}
			if s.checks[1].Verdict != "clean" || c.BatchCalls != 1 {
				t.Fatal("wrong order or resubmission")
			}
			want := "api_error"
			if kind == "expired" || kind == "canceled" {
				want = kind
			}
			if kind == "succeeded" {
				if len(s.checks) != 2 {
					t.Fatal("lost successful check")
				}
			} else if len(s.checks) != 1 || s.attempts[1].Reason != want {
				t.Fatalf("attempt=%+v", s.attempts[1])
			}
		})
	}
}
func TestCheck_DryRunWritesNothingAndCountsTokens(t *testing.T) {
	r, s, c, d := fixture(t)
	o := options()
	o.DryRun = true
	summary, err := r.Run(context.Background(), d, o)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.attempts) != 0 || len(s.checks) != 0 || c.DirectCalls != 0 || c.BatchCalls != 0 || c.CountCalls != 1 || summary.EstimateMicroUSD != 320200 {
		t.Fatalf("summary=%+v", summary)
	}
}
func TestCheck_CompletedAndPendingDoNotSpendAgain(t *testing.T) {
	for _, batch := range []bool{true, false} {
		t.Run(fmt.Sprint(batch), func(t *testing.T) {
			r, _, c, d := fixture(t)
			o := options()
			o.Batch = batch
			if err := run(t, r, d, o); err != nil {
				t.Fatal(err)
			}
			if err := run(t, r, d, o); err != nil {
				t.Fatal(err)
			}
			if batch && c.BatchCalls != 1 || !batch && c.DirectCalls != 1 {
				t.Fatal("duplicate paid request")
			}
		})
	}
}
func TestCheck_FailedUsageReducesRemainingBudget(t *testing.T) {
	r, s, c, d := fixture(t)
	c.Response.Stop = "refusal"
	c.Response.Usage.Output = 32000
	o := options()
	o.MaxUSD = .33
	if err := run(t, r, d, o); err != nil {
		t.Fatal(err)
	}
	if err := run(t, r, d, o); err != ErrBudget {
		t.Fatalf("error=%v", err)
	}
	if len(s.attempts) != 1 || c.DirectCalls != 1 {
		t.Fatal("failed usage did not consume budget")
	}
}

func TestCheck_BatchAPIErrorRecordsFailedIntent(t *testing.T) {
	r, s, c, d := fixture(t)
	o := options()
	o.Batch = true
	c.CreateError = &APIError{Status: 400, RequestID: "req_invalid", Reason: "api_error"}
	if err := run(t, r, d, o); err == nil {
		t.Fatal("missing provider error")
	}
	if len(s.attempts) != 1 || s.attempts[0].State != "failed" || s.attempts[0].Reason != "api_error" {
		t.Fatalf("attempt=%+v", s.attempts)
	}
}

func TestCheck_MissingPDFAndPageLimit(t *testing.T) {
	for _, missing := range []bool{true, false} {
		t.Run(fmt.Sprint(missing), func(t *testing.T) {
			r, s, c, d := fixture(t)
			reason := "over_check_limit"
			if missing {
				d[0].PDFPath += "-missing"
				reason = "pdf_missing"
			} else {
				r.CountPages = func(context.Context, []byte) (int, error) { return 601, nil }
			}
			if err := run(t, r, d, options()); err != nil {
				t.Fatal(err)
			}
			if c.CountCalls != 0 || c.DirectCalls != 0 || len(s.checks) != 0 || s.attempts[0].Reason != reason {
				t.Fatal(s.attempts)
			}
		})
	}
}

func TestCheck_UnexpectedCacheUsageStopsSpending(t *testing.T) {
	r, _, c, d := fixture(t)
	second := d[0]
	second.Entry.ID = 2
	d = append(d, second)
	c.Response.Usage.CacheCreation = 10
	if err := run(t, r, d, options()); err == nil {
		t.Fatal("unverified cache price did not stop spending")
	}
	if c.DirectCalls != 1 {
		t.Fatal("sent more work with unknown spend")
	}
}

func TestCheck_BatchNULFailsOnlyOneDocument(t *testing.T) {
	r, s, c, docs := fixture(t)
	o := options()
	o.Batch = true
	second := docs[0]
	second.Entry.ID = 2
	docs = append(docs, second)
	if err := run(t, r, docs, o); err != nil {
		t.Fatal(err)
	}
	bad := c.Response
	bad.Text = `{"same_document":true,"pages":10,"items":[{"kind":"date","page":1,"ocr":"public OCR text","pdf":"different\u0000","note":"x"}]}`
	c.BatchEnded = true
	c.Results = []BatchResult{{CustomID: customID(s.attempts[0]), Type: "succeeded", Response: bad}, {CustomID: customID(s.attempts[1]), Type: "succeeded", Response: c.Response}}
	summary, err := r.Run(context.Background(), docs, o)
	if err != nil {
		t.Fatal(err)
	}
	if s.attempts[0].State != "failed" || s.attempts[0].Reason != "schema:nul" || s.attempts[0].Usage.Input != 100 || s.attempts[0].Usage.Output != 100 || s.attempts[0].Usage.CostMicroUSD != 600 || !s.attempts[0].Usage.Final {
		t.Fatalf("attempt=%+v", s.attempts[0])
	}
	if _, ok := s.checks[1]; ok || len(s.checks) != 1 || s.checks[2].Verdict != "clean" || summary.Failed != 1 || summary.Complete != 1 {
		t.Fatalf("checks=%+v summary=%+v", s.checks, summary)
	}
}
