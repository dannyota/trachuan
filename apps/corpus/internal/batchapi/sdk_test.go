package batchapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go/v2"
	"github.com/anthropics/anthropic-sdk-go/v2/option"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestSDK_CountAndSubmitIdenticalAdaptiveRequest(t *testing.T) {
	calls := 0
	s := SDK{Client: anthropic.NewClient(option.WithoutEnvironmentDefaults(), option.WithAPIKey("synthetic"), option.WithMaxRetries(0), option.WithHTTPClient(&http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		b, _ := io.ReadAll(r.Body)
		var body map[string]any
		if err := json.Unmarshal(b, &body); err != nil {
			t.Fatal(err)
		}
		response := `{"input_tokens":42}`
		if strings.HasSuffix(r.URL.Path, "/batches") {
			item := body["requests"].([]any)[0].(map[string]any)
			if item["custom_id"] != "abcdef-000000" {
				t.Fatal("lost identity")
			}
			body = item["params"].(map[string]any)
			if body["max_tokens"] != float64(4096) {
				t.Fatal("lost output bound")
			}
			response = `{"id":"batch_synthetic","processing_status":"in_progress"}`
		}
		if body["thinking"].(map[string]any)["type"] != "adaptive" || body["output_config"].(map[string]any)["effort"] != "medium" {
			t.Fatal("lost thinking or effort")
		}
		if _, ok := body["temperature"]; ok {
			t.Fatal("sampling parameter")
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(response))}, nil
	})}))}
	p := anthropic.MessageNewParams{Model: "claude-haiku-5-5", MaxTokens: 4096, Messages: []anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock("synthetic legal text"))}, Thinking: anthropic.ThinkingConfigParamUnion{OfAdaptive: &anthropic.ThinkingConfigAdaptiveParam{}}, OutputConfig: anthropic.OutputConfigParam{Effort: "medium", Format: anthropic.JSONOutputFormatParam{Schema: map[string]any{"type": "object"}}}}
	n, err := s.CountTokens(context.Background(), p)
	if err != nil || n != 42 {
		t.Fatalf("count %d error %v", n, err)
	}
	b, err := s.Create(context.Background(), []Request{{CustomID: "abcdef-000000", Params: p}})
	if err != nil || b.ID != "batch_synthetic" || calls != 2 {
		t.Fatalf("batch %s calls %d error %v", b.ID, calls, err)
	}
}

func TestSDK_CancelUsesProviderEndpoint(t *testing.T) {
	called := false
	s := SDK{Client: anthropic.NewClient(option.WithoutEnvironmentDefaults(), option.WithAPIKey("synthetic"), option.WithMaxRetries(0), option.WithHTTPClient(&http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		called = true
		if r.Method != "POST" || !strings.HasSuffix(r.URL.Path, "/batches/batch_synthetic/cancel") {
			t.Fatal("wrong cancellation endpoint")
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"id":"batch_synthetic","processing_status":"canceling"}`))}, nil
	})}))}
	if err := s.Cancel(context.Background(), "batch_synthetic"); err != nil || !called {
		t.Fatalf("called %v error %v", called, err)
	}
}
