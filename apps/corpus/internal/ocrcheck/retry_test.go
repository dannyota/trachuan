package ocrcheck

import (
	"context"
	"testing"
)

func TestCheck_RetryLimit(t *testing.T) {
	r, _, c, docs := fixture(t)
	c.Response.Text = `{"same_document":true,"pages":0,"items":[]}`
	for i := 0; i < 2; i++ {
		if err := run(t, r, docs, options()); err != nil {
			t.Fatal(err)
		}
	}
	s, err := r.Run(context.Background(), docs, options())
	if err != nil || c.DirectCalls != 2 || s.SkippedRetryLimit != 1 || len(s.Documents) != 1 || s.Documents[0].Outcome != "retry_limit" {
		t.Fatalf("calls=%d summary=%+v err=%v", c.DirectCalls, s, err)
	}
	o := options()
	o.DryRun = true
	s, err = r.Run(context.Background(), docs, o)
	if err != nil || s.Estimated != 0 || s.EstimateMicroUSD != 0 || s.SkippedRetryLimit != 1 || len(s.Documents) != 1 || s.Documents[0].Outcome != "retry_limit" {
		t.Fatalf("summary=%+v err=%v", s, err)
	}
}

func TestCheck_PreRequestFailureIsSkipped(t *testing.T) {
	r, repo, c, docs := fixture(t)
	r.CountPages = func(context.Context, []byte) (int, error) { return 501, nil }
	if err := run(t, r, docs, options()); err != nil {
		t.Fatal(err)
	}
	if repo.attempts[0].Reason != "over_check_limit" {
		t.Fatal(repo.attempts)
	}
	s, err := r.Run(context.Background(), docs, options())
	if err != nil || c.DirectCalls != 0 || len(s.Documents) != 1 || s.Documents[0].Outcome != "retry_limit" {
		t.Fatalf("summary=%+v err=%v", s, err)
	}
}

func TestCheck_RetryOverrideAndCompleted(t *testing.T) {
	r, repo, c, docs := fixture(t)
	c.Response.Text = `{"same_document":true,"pages":0,"items":[]}`
	for i := 0; i < 2; i++ {
		if err := run(t, r, docs, options()); err != nil {
			t.Fatal(err)
		}
	}
	o := options()
	o.RetryFailed = true
	c.Response.Text = `{"same_document":true,"pages":10,"items":[]}`
	if err := run(t, r, docs, o); err != nil {
		t.Fatal(err)
	}
	if c.DirectCalls != 3 || len(repo.checks) != 1 {
		t.Fatalf("calls=%d checks=%v", c.DirectCalls, repo.checks)
	}
	if err := run(t, r, docs, o); err != nil {
		t.Fatal(err)
	}
	if c.DirectCalls != 3 {
		t.Fatal("completed check was submitted again")
	}
}

func TestCheck_UnbilledFailureDoesNotCount(t *testing.T) {
	r, _, c, docs := fixture(t)
	r.CountPages = func(context.Context, []byte) (int, error) { return 0, nil }
	if err := run(t, r, docs, options()); err != nil {
		t.Fatal(err)
	}
	r.CountPages = func(context.Context, []byte) (int, error) { return 10, nil }
	c.Response.Text = `{"same_document":true,"pages":0,"items":[]}`
	o := options()
	o.RetryFailed = true
	if err := run(t, r, docs, o); err != nil {
		t.Fatal(err)
	}
	if err := run(t, r, docs, options()); err != nil {
		t.Fatal(err)
	}
	if c.DirectCalls != 2 {
		t.Fatalf("calls=%d", c.DirectCalls)
	}
}

func TestCheck_DryRunDoesNotEstimateSubmitted(t *testing.T) {
	r, _, _, docs := fixture(t)
	o := options()
	o.Batch = true
	if err := run(t, r, docs, o); err != nil {
		t.Fatal(err)
	}
	o.DryRun = true
	s, err := r.Run(context.Background(), docs, o)
	if err != nil || s.Estimated != 0 || s.EstimateMicroUSD != 0 {
		t.Fatalf("summary=%+v err=%v", s, err)
	}
}
