package ocrcheck

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/anthropics/anthropic-sdk-go/v2"
	"github.com/anthropics/anthropic-sdk-go/v2/option"
)

// SDKClient uses only the official Claude SDK and never returns its raw errors.
type SDKClient struct{ client anthropic.Client }

// NewSDKClient requires the environment credential before constructing the SDK,
// preventing its profile and federation fallbacks. The SDK reads the key itself.
func NewSDKClient() (*SDKClient, error) {
	if value, ok := os.LookupEnv("ANTHROPIC_API_KEY"); !ok || value == "" {
		return nil, fmt.Errorf("ANTHROPIC_API_KEY is required")
	}
	return &SDKClient{client: anthropic.NewClient(option.WithMaxRetries(0), option.WithRequestTimeout(10*time.Minute))}, nil
}

// CountTokens counts the exact PDF, text, instruction and output schema.
func (s *SDKClient) CountTokens(ctx context.Context, r Request) (int64, error) {
	p := messageParams(r)
	v, err := s.client.Messages.CountTokens(ctx, anthropic.MessageCountTokensParams{Model: p.Model, Messages: p.Messages, OutputConfig: p.OutputConfig})
	if err != nil {
		return 0, sanitizeSDKError(err)
	}
	return v.InputTokens, nil
}

// Direct streams long requests and keeps partial usage on all error paths.
func (s *SDKClient) Direct(ctx context.Context, r Request) (Response, error) {
	stream := s.client.Messages.NewStreaming(ctx, messageParams(r))
	defer func() { _ = stream.Close() }()
	var message anthropic.Message
	ended := false
	finalUsage := false
	response := func() Response { v := fromMessage(message); v.FinalUsage = finalUsage; return v }
	for stream.Next() {
		event := stream.Current()
		if err := message.Accumulate(event); err != nil {
			return response(), sanitizeSDKError(err)
		}
		if event.Type == "message_delta" && event.Delta.StopReason != "" && event.Usage.JSON.OutputTokens.Valid() {
			finalUsage = true
		}
		if event.Type == "message_stop" {
			ended = true
		}
	}
	result := response()
	if err := stream.Err(); err != nil {
		return result, sanitizeSDKError(err)
	}
	if !ended {
		return result, &APIError{Reason: "api_error"}
	}
	return result, nil
}
func fromMessage(m anthropic.Message) Response {
	r := Response{FinalUsage: m.Usage.JSON.InputTokens.Valid() && m.Usage.JSON.OutputTokens.Valid(), Stop: string(m.StopReason), Usage: Usage{Input: m.Usage.InputTokens, Output: m.Usage.OutputTokens, CacheRead: m.Usage.CacheReadInputTokens, CacheCreation: m.Usage.CacheCreationInputTokens}}
	for _, c := range m.Content {
		if c.Type == "text" {
			r.Text += c.Text
		}
	}
	return r
}
func batchRequest(r Request) anthropic.MessageBatchNewParamsRequest {
	p := messageParams(r)
	return anthropic.MessageBatchNewParamsRequest{CustomID: r.CustomID, Params: anthropic.MessageBatchNewParamsRequestParams{Model: p.Model, Messages: p.Messages, MaxTokens: p.MaxTokens, OutputConfig: p.OutputConfig}}
}

// CreateBatch submits exactly once. A lost response must be reconciled.
func (s *SDKClient) CreateBatch(ctx context.Context, requests []Request) (Batch, error) {
	params := anthropic.MessageBatchNewParams{Requests: make([]anthropic.MessageBatchNewParamsRequest, len(requests))}
	for i, r := range requests {
		params.Requests[i] = batchRequest(r)
	}
	b, err := s.client.Messages.Batches.New(ctx, params)
	if err != nil {
		return Batch{}, sanitizeSDKError(err)
	}
	return fromBatch(*b), nil
}

// GetBatch reads status without waiting for completion.
func (s *SDKClient) GetBatch(ctx context.Context, id string) (Batch, error) {
	b, err := s.client.Messages.Batches.Get(ctx, id, anthropic.MessageBatchGetParams{})
	if err != nil {
		return Batch{}, sanitizeSDKError(err)
	}
	return fromBatch(*b), nil
}

// BatchResults downloads typed JSONL through the SDK.
func (s *SDKClient) BatchResults(ctx context.Context, id string) ([]BatchResult, error) {
	stream := s.client.Messages.Batches.ResultsStreaming(ctx, id, anthropic.MessageBatchResultsParams{})
	defer func() { _ = stream.Close() }()
	var results []BatchResult
	for stream.Next() {
		v := stream.Current()
		response := fromMessage(v.Result.Message)
		if v.Result.Type != "succeeded" {
			response = Response{Usage: Usage{RequestID: v.Result.Error.RequestID}}
		}
		results = append(results, BatchResult{CustomID: v.CustomID, Type: v.Result.Type, Response: response})
	}
	if err := stream.Err(); err != nil {
		return nil, sanitizeSDKError(err)
	}
	return results, nil
}

// ListBatches scans all pages and filters the submission's possible time window.
func (s *SDKClient) ListBatches(ctx context.Context, after, before time.Time) ([]Batch, error) {
	pager := s.client.Messages.Batches.ListAutoPaging(ctx, anthropic.MessageBatchListParams{})
	var batches []Batch
	for pager.Next() {
		b := pager.Current()
		if !b.CreatedAt.Before(after) && !b.CreatedAt.After(before) {
			batches = append(batches, fromBatch(b))
		}
	}
	if err := pager.Err(); err != nil {
		return nil, sanitizeSDKError(err)
	}
	return batches, nil
}
func fromBatch(b anthropic.MessageBatch) Batch {
	return Batch{ID: b.ID, Ended: b.ProcessingStatus == anthropic.MessageBatchProcessingStatusEnded, CreatedAt: b.CreatedAt}
}
func sanitizeSDKError(err error) error {
	if err == nil {
		return nil
	}
	var apiErr *anthropic.Error
	if errors.As(err, &apiErr) {
		return &APIError{Status: apiErr.StatusCode, RequestID: apiErr.RequestID, Reason: "api_error"}
	}
	var timeout net.Error
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) || (errors.As(err, &timeout) && timeout.Timeout()) {
		return &APIError{Reason: "timeout"}
	}
	return &APIError{Reason: "api_error"}
}
