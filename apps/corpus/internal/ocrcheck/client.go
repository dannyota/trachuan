// Package ocrcheck compares immutable OCR text with its source PDF.
package ocrcheck

import (
	"context"
	"fmt"
	"time"
)

// Request carries public legal evidence only. It must never be logged.
type Request struct {
	Model, Effort, CustomID string
	MediaType               string
	Images                  [][]byte
	Text                    []byte
}

// Response contains the checker output and usage, including on failure.
// FinalUsage is true only after authoritative final token totals arrive.
type Response struct {
	Stop, Text string
	FinalUsage bool
	Usage      Usage
}

// Batch identifies a durable provider submission.
type Batch struct {
	ID        string
	Ended     bool
	CreatedAt time.Time
}

// BatchResult pairs an arbitrary-order outcome with its request identity.
type BatchResult struct {
	CustomID, Type string
	Response       Response
}

// Client isolates the official SDK from offline tests.
type Client interface {
	CountTokens(context.Context, Request) (int64, error)
	Direct(context.Context, Request) (Response, error)
	CreateBatch(context.Context, []Request) (Batch, error)
	GetBatch(context.Context, string) (Batch, error)
	BatchResults(context.Context, string) ([]BatchResult, error)
	ListBatches(context.Context, time.Time, time.Time) ([]Batch, error)
}

// APIError keeps only safe provider metadata, never the SDK request or body.
type APIError struct {
	Status            int
	RequestID, Reason string
}

// Error returns status and request identity without the provider body.
func (e *APIError) Error() string {
	return fmt.Sprintf("Claude API status=%d request_id=%s reason=%s", e.Status, e.RequestID, e.Reason)
}
