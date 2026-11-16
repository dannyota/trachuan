package ocrcheck

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"syscall"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/batchapi"

	"github.com/anthropics/anthropic-sdk-go/v2"
	"github.com/anthropics/anthropic-sdk-go/v2/option"
	"golang.org/x/net/http2"
)

type SDKClient struct{ client anthropic.Client }

func NewSDKClient() (*SDKClient, error) {
	if value, ok := os.LookupEnv("ANTHROPIC_API_KEY"); !ok || value == "" {
		return nil, fmt.Errorf("ANTHROPIC_API_KEY is required")
	}
	return &SDKClient{client: anthropic.NewClient(option.WithMaxRetries(0), option.WithRequestTimeout(10*time.Minute))}, nil
}

func (s *SDKClient) CountTokens(ctx context.Context, r Request) (int64, error) {
	v, err := (batchapi.SDK{Client: s.client}).CountTokens(ctx, messageParams(r))
	if err != nil {
		return 0, sanitizeSDKError(err)
	}
	return v, nil
}

func (s *SDKClient) Direct(ctx context.Context, r Request) (Response, error) {
	events := 0
	stream := s.client.Messages.NewStreaming(ctx, messageParams(r), option.WithMiddleware(func(req *http.Request, next option.MiddlewareNext) (*http.Response, error) {
		resp, err := next(req)
		if err == nil && resp != nil && resp.Body != nil {
			resp.Body = countingStreamBody{ReadCloser: resp.Body, events: &events}
		}
		return resp, err
	}))
	defer func() { _ = stream.Close() }()
	var message anthropic.Message
	ended := false
	finalUsage := false
	response := func() Response {
		v := fromMessage(message)
		v.FinalUsage = finalUsage
		v.Usage.ResponseEvents = events
		return v
	}
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
		return result, &APIError{Reason: "api_error", Class: "incomplete_stream"}
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
	return batchapi.Parameters(batchapi.Request{CustomID: r.CustomID, Params: messageParams(r)})
}

func (s *SDKClient) CreateBatch(ctx context.Context, requests []Request) (Batch, error) {
	params := make([]batchapi.Request, len(requests))
	for i, r := range requests {
		params[i] = batchapi.Request{CustomID: r.CustomID, Params: messageParams(r)}
	}
	b, err := (batchapi.SDK{Client: s.client}).Create(ctx, params)
	if err != nil {
		return Batch{}, sanitizeSDKError(err)
	}
	return fromBatch(b), nil
}

func (s *SDKClient) GetBatch(ctx context.Context, id string) (Batch, error) {
	b, err := (batchapi.SDK{Client: s.client}).Get(ctx, id)
	if err != nil {
		return Batch{}, sanitizeSDKError(err)
	}
	return fromBatch(b), nil
}

func (s *SDKClient) BatchResults(ctx context.Context, id string) ([]BatchResult, error) {
	values, err := (batchapi.SDK{Client: s.client}).Results(ctx, id)
	if err != nil {
		return nil, sanitizeSDKError(err)
	}
	var results []BatchResult
	for _, v := range values {
		response := fromMessage(v.Message)
		if v.Type != "succeeded" {
			response = Response{Usage: Usage{RequestID: v.RequestID}}
			if v.Type == "errored" {
				response.Usage.APIErrorType = providerErrorType(v.ErrorJSON)
			}
		}
		results = append(results, BatchResult{CustomID: v.CustomID, Type: v.Type, Response: response})
	}
	return results, nil
}

func (s *SDKClient) ListBatches(ctx context.Context, after, before time.Time) ([]Batch, error) {
	values, err := (batchapi.SDK{Client: s.client}).List(ctx, after, before)
	if err != nil {
		return nil, sanitizeSDKError(err)
	}
	var batches []Batch
	for _, b := range values {
		batches = append(batches, fromBatch(b))
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
	reason := "api_error"
	var apiErr *anthropic.Error
	var timeout net.Error
	if !errors.As(err, &apiErr) && (errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) || (errors.As(err, &timeout) && timeout.Timeout())) {
		reason = "timeout"
	}
	result := &APIError{Reason: reason, Class: errorClass(err)}
	if apiErr != nil {
		result.Status = apiErr.StatusCode
		result.RequestID = apiErr.RequestID
		result.ProviderType = providerErrorType(apiErr.RawJSON())
	}
	inner := err
	for {
		next := errors.Unwrap(inner)
		if next == nil {
			break
		}
		inner = next
	}
	result.Type = fmt.Sprintf("%T", inner)
	return result
}

func errorClass(err error) string {
	var api *anthropic.Error
	var timeout net.Error
	var stream http2.StreamError
	var streamPtr *http2.StreamError
	var goaway http2.GoAwayError     //nolint:staticcheck // The SDK transport can still return this deprecated type.
	var goawayPtr *http2.GoAwayError //nolint:staticcheck // The SDK transport can still return this deprecated type.
	var record tls.RecordHeaderError
	var recordPtr *tls.RecordHeaderError
	var alert tls.AlertError
	var cert *tls.CertificateVerificationError
	var dns *net.DNSError
	switch {
	case errors.As(err, &api):
		return "http_status"
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, context.Canceled):
		return "canceled"
	case errors.As(err, &timeout) && timeout.Timeout():
		return "timeout"
	case errors.Is(err, io.EOF), errors.Is(err, io.ErrUnexpectedEOF):
		return "eof"
	case errors.Is(err, syscall.ECONNRESET):
		return "connection_reset"
	case errors.Is(err, syscall.EPIPE):
		return "broken_pipe"
	case errors.As(err, &stream), errors.As(err, &streamPtr), errors.As(err, &goaway), errors.As(err, &goawayPtr):
		return "http2_stream"
	case errors.As(err, &record), errors.As(err, &recordPtr), errors.As(err, &alert), errors.As(err, &cert):
		return "tls"
	case errors.As(err, &dns):
		return "dns"
	default:
		return "other"
	}
}

func providerErrorType(body string) string {
	var envelope struct {
		Error struct {
			Type json.RawMessage `json:"type"`
		} `json:"error"`
	}
	if json.Unmarshal([]byte(body), &envelope) != nil || len(envelope.Error.Type) == 0 {
		return ""
	}
	var value string
	if json.Unmarshal(envelope.Error.Type, &value) == nil {
		switch value {
		case "invalid_request_error", "authentication_error", "permission_error", "not_found_error",
			"request_too_large", "rate_limit_error", "api_error", "overloaded_error", "billing_error", "timeout_error":
			return value
		}
	}
	return "other"
}
