package ocrcheck

import (
	"context"
	"fmt"
	"time"
)

// FakeClient supplies deterministic responses without a credential or network.
// Runner invokes clients serially, so this fake needs no synchronization.
type FakeClient struct {
	Counted                             []Request
	Response                            Response
	Tokens                              int64
	DirectError, CreateError            error
	DirectCalls, BatchCalls, CountCalls int
	BatchEnded                          bool
	Results                             []BatchResult
	Batches                             []Batch
}

// CountTokens returns the configured input count.
func (f *FakeClient) CountTokens(_ context.Context, r Request) (int64, error) {
	f.Counted = append(f.Counted, r)
	f.CountCalls++
	return f.Tokens, nil
}

// Direct returns the configured message and error.
func (f *FakeClient) Direct(context.Context, Request) (Response, error) {
	f.DirectCalls++
	return f.Response, f.DirectError
}

// CreateBatch returns a unique local batch identity.
func (f *FakeClient) CreateBatch(context.Context, []Request) (Batch, error) {
	f.BatchCalls++
	return Batch{ID: fmt.Sprintf("batch-%d", f.BatchCalls)}, f.CreateError
}

// GetBatch returns the configured processing status.
func (f *FakeClient) GetBatch(_ context.Context, id string) (Batch, error) {
	return Batch{ID: id, Ended: f.BatchEnded}, nil
}

// BatchResults returns recorded outcomes.
func (f *FakeClient) BatchResults(context.Context, string) ([]BatchResult, error) {
	return f.Results, nil
}

// ListBatches returns recorded reconciliation candidates.
func (f *FakeClient) ListBatches(context.Context, time.Time, time.Time) ([]Batch, error) {
	return f.Batches, nil
}
