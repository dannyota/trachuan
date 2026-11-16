package batchapi

import (
	"context"
	"fmt"
	"time"

	"github.com/anthropics/anthropic-sdk-go/v2"
)

type Request struct {
	CustomID string
	Params   anthropic.MessageNewParams
}

type Result struct {
	CustomID, Type       string
	Message              anthropic.Message
	RequestID, ErrorJSON string
}

type SDK struct{ Client anthropic.Client }

func Parameters(r Request) anthropic.MessageBatchNewParamsRequest {
	p := r.Params
	return anthropic.MessageBatchNewParamsRequest{CustomID: r.CustomID, Params: anthropic.MessageBatchNewParamsRequestParams{Model: p.Model, Messages: p.Messages, MaxTokens: p.MaxTokens, OutputConfig: p.OutputConfig, Thinking: p.Thinking}}
}

func (s SDK) CountTokens(ctx context.Context, p anthropic.MessageNewParams) (int64, error) {
	v, err := s.Client.Messages.CountTokens(ctx, anthropic.MessageCountTokensParams{Model: p.Model, Messages: p.Messages, OutputConfig: p.OutputConfig, Thinking: p.Thinking})
	if err != nil {
		return 0, fmt.Errorf("count batch input: %w", err)
	}
	return v.InputTokens, nil
}

func (s SDK) Create(ctx context.Context, requests []Request) (anthropic.MessageBatch, error) {
	p := anthropic.MessageBatchNewParams{Requests: make([]anthropic.MessageBatchNewParamsRequest, len(requests))}
	for i, r := range requests {
		p.Requests[i] = Parameters(r)
	}
	b, err := s.Client.Messages.Batches.New(ctx, p)
	if err != nil {
		return anthropic.MessageBatch{}, fmt.Errorf("create batch: %w", err)
	}
	return *b, nil
}

func (s SDK) Get(ctx context.Context, id string) (anthropic.MessageBatch, error) {
	b, err := s.Client.Messages.Batches.Get(ctx, id, anthropic.MessageBatchGetParams{})
	if err != nil {
		return anthropic.MessageBatch{}, fmt.Errorf("get batch: %w", err)
	}
	return *b, nil
}

func (s SDK) Results(ctx context.Context, id string) ([]Result, error) {
	stream := s.Client.Messages.Batches.ResultsStreaming(ctx, id, anthropic.MessageBatchResultsParams{})
	defer func() { _ = stream.Close() }()
	var results []Result
	for stream.Next() {
		v := stream.Current()
		results = append(results, Result{CustomID: v.CustomID, Type: v.Result.Type, Message: v.Result.Message, RequestID: v.Result.Error.RequestID, ErrorJSON: v.Result.Error.RawJSON()})
	}
	if err := stream.Err(); err != nil {
		return nil, fmt.Errorf("read batch results: %w", err)
	}
	return results, nil
}

func (s SDK) List(ctx context.Context, after, before time.Time) ([]anthropic.MessageBatch, error) {
	pager := s.Client.Messages.Batches.ListAutoPaging(ctx, anthropic.MessageBatchListParams{})
	var batches []anthropic.MessageBatch
	for pager.Next() {
		b := pager.Current()
		if !b.CreatedAt.Before(after) && !b.CreatedAt.After(before) {
			batches = append(batches, b)
		}
	}
	if err := pager.Err(); err != nil {
		return nil, fmt.Errorf("list batches: %w", err)
	}
	return batches, nil
}

func (s SDK) Cancel(ctx context.Context, id string) error {
	_, err := s.Client.Messages.Batches.Cancel(ctx, id, anthropic.MessageBatchCancelParams{})
	if err != nil {
		return fmt.Errorf("cancel batch: %w", err)
	}
	return nil
}
