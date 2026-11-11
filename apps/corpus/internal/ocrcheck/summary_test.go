package ocrcheck

import (
	"context"
	"testing"
)

func TestCheck_SummaryRetainsPagesOnUncertainBilling(t *testing.T) {
	r, _, c, d := fixture(t)
	c.Response = Response{Usage: Usage{Input: 12, Output: 3}}
	c.DirectError = context.DeadlineExceeded
	s, err := r.Run(context.Background(), d, options())
	if err == nil || len(s.Documents) != 1 || s.Documents[0].Pages != 10 || s.Documents[0].InputTokens != 12 || s.Documents[0].Outcome != "uncertain:timeout" {
		t.Fatalf("summary=%+v error=%v", s.Documents, err)
	}
}
func TestCheck_BatchSubmissionSummarizesEachDocument(t *testing.T) {
	r, _, _, d := fixture(t)
	o := options()
	o.Batch = true
	s, err := r.Run(context.Background(), d, o)
	if err != nil || len(s.Documents) != 1 || s.Documents[0].Pages != 10 || s.Documents[0].InputTokens != 100 || s.Documents[0].Outcome != "submitted" {
		t.Fatalf("summary=%+v error=%v", s.Documents, err)
	}
}

func TestCheck_PendingBatchSummarizesEachDocument(t *testing.T) {
	r, _, _, d := fixture(t)
	o := options()
	o.Batch = true
	if _, err := r.Run(context.Background(), d, o); err != nil {
		t.Fatal(err)
	}
	s, err := r.Run(context.Background(), d, o)
	if err != nil || len(s.Documents) != 1 || s.Documents[0].Pages != 10 || s.Documents[0].Outcome != "submitted" {
		t.Fatalf("summary=%+v error=%v", s.Documents, err)
	}
}
