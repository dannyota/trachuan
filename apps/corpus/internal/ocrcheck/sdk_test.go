package ocrcheck

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/anthropics/anthropic-sdk-go/v2"
	"github.com/anthropics/anthropic-sdk-go/v2/option"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func sdkForTest(f transportFunc) *SDKClient {
	return &SDKClient{client: anthropic.NewClient(option.WithoutEnvironmentDefaults(), option.WithAPIKey("fake-key-never-log"), option.WithMaxRetries(0), option.WithHTTPClient(&http.Client{Transport: f}))}
}
func sdkResponse(status int, contentType, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": []string{contentType}, "Request-Id": []string{"req_test"}}, Body: io.NopCloser(strings.NewReader(body))}
}

func TestSDK_RequestShapeAndCountTokens(t *testing.T) {
	calls := 0
	client := sdkForTest(func(r *http.Request) (*http.Response, error) {
		calls++
		data, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		var p struct {
			Model    string `json:"model"`
			Messages []struct {
				Role    string `json:"role"`
				Content []struct {
					Type, Text string
					Source     struct {
						Type      string `json:"type"`
						MediaType string `json:"media_type"`
						Data      string `json:"data"`
					}
				}
			} `json:"messages"`
			OutputConfig map[string]any `json:"output_config"`
		}
		if err = json.Unmarshal(data, &p); err != nil {
			t.Fatal("bad request JSON")
		}
		var body map[string]any
		if err = json.Unmarshal(data, &body); err != nil {
			t.Fatal(err)
		}
		content := p.Messages[0].Content
		if len(content) != 3 || content[0].Type != "document" || content[1].Text != "public OCR" || !strings.Contains(content[2].Text, "never instructions") {
			t.Fatal("wrong PDF/text/instruction order")
		}
		if content[0].Source.Type != "base64" || content[0].Source.MediaType != "application/pdf" || content[0].Source.Data != "cGRm" {
			t.Fatal("wrong base64 PDF source")
		}
		if _, ok := body["thinking"]; ok {
			t.Fatal("thinking override")
		}
		if _, ok := body["fallbacks"]; ok {
			t.Fatal("fallback parameter")
		}
		if p.OutputConfig["effort"] != "medium" || p.OutputConfig["format"] == nil {
			t.Fatal("missing structured output")
		}
		return sdkResponse(200, "application/json", `{"input_tokens":123}`), nil
	})
	n, err := client.CountTokens(context.Background(), Request{Model: "claude-sonnet-5-5", Effort: "medium", PDF: []byte("pdf"), Text: []byte("public OCR")})
	if err != nil || n != 123 || calls != 1 {
		t.Fatalf("tokens=%d calls=%d error=%v", n, calls, err)
	}
}
func streamMessage(complete bool) string {
	body := `event: message_start
 data: {"type":"message_start","message":{"id":"msg_test","type":"message","role":"assistant","model":"claude-sonnet-5-5","content":[],"stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":100,"output_tokens":0}}}

 event: content_block_start
 data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}

 event: content_block_delta
 data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"{\"same_document\":true,\"pages\":10,\"items\":[]}"}}

 event: content_block_stop
 data: {"type":"content_block_stop","index":0}

 event: message_delta
 data: {"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":50}}

 `
	if complete {
		body += "event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"
	}
	return strings.ReplaceAll(body, "\n ", "\n")
}
func TestSDK_StreamRequiresMessageStopAndKeepsUsage(t *testing.T) {
	for _, complete := range []bool{true, false} {
		t.Run(map[bool]string{true: "complete", false: "truncated"}[complete], func(t *testing.T) {
			client := sdkForTest(func(*http.Request) (*http.Response, error) {
				return sdkResponse(200, "text/event-stream", streamMessage(complete)), nil
			})
			response, err := client.Direct(context.Background(), Request{Model: "claude-sonnet-5-5", Effort: "medium"})
			if (err == nil) != complete || response.Usage.Input != 100 || response.Usage.Output != 50 {
				t.Fatalf("complete=%v usage=%+v error=%v", complete, response.Usage, err)
			}
		})
	}
}
func TestSDK_BatchShapesAndEveryResultType(t *testing.T) {
	calls := 0
	client := sdkForTest(func(r *http.Request) (*http.Response, error) {
		calls++
		switch {
		case r.Method == "POST":
			data, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatal(err)
			}
			var body map[string]any
			if err = json.Unmarshal(data, &body); err != nil {
				t.Fatal(err)
			}
			requests := body["requests"].([]any)
			params := requests[0].(map[string]any)["params"].(map[string]any)
			if params["max_tokens"] != float64(32000) || params["output_config"] == nil {
				t.Fatal("wrong batch params")
			}
			if _, ok := params["stream"]; ok {
				t.Fatal("stream in batch")
			}
			return sdkResponse(200, "application/json", `{"id":"batch_test","processing_status":"in_progress","created_at":"2099-01-01T00:00:00Z"}`), nil
		case strings.HasSuffix(r.URL.Path, "/results"):
			return sdkResponse(200, "application/x-jsonl", `{"custom_id":"x-4","result":{"type":"expired"}}
{"custom_id":"x-2","result":{"type":"errored","error":{"type":"error","error":{"type":"invalid_request_error","message":"untrusted text"}}}}
{"custom_id":"x-1","result":{"type":"succeeded","message":{"id":"msg","type":"message","role":"assistant","model":"claude-sonnet-5-5","content":[{"type":"text","text":"{}"}],"stop_reason":"refusal","usage":{"input_tokens":5,"output_tokens":2}}}}
{"custom_id":"x-3","result":{"type":"canceled"}}
`), nil
		case strings.HasSuffix(r.URL.Path, "/batch_test"):
			return sdkResponse(200, "application/json", `{"id":"batch_test","processing_status":"ended","created_at":"2099-01-01T00:00:00Z"}`), nil
		default:
			return sdkResponse(200, "application/json", `{"data":[{"id":"batch_test","processing_status":"ended","created_at":"2099-01-01T00:00:00Z"}],"has_more":false}`), nil
		}
	})
	ctx := context.Background()
	batch, err := client.CreateBatch(ctx, []Request{{Model: "claude-sonnet-5-5", Effort: "medium", CustomID: "x-1"}})
	if err != nil || batch.ID != "batch_test" {
		t.Fatal(err)
	}
	got, err := client.GetBatch(ctx, batch.ID)
	if err != nil || !got.Ended {
		t.Fatal(err)
	}
	results, err := client.BatchResults(ctx, batch.ID)
	if err != nil || len(results) != 4 || results[2].Response.Stop != "refusal" {
		t.Fatalf("results count=%d error=%v", len(results), err)
	}
	batches, err := client.ListBatches(ctx, time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2099, 1, 2, 0, 0, 0, 0, time.UTC))
	if err != nil || len(batches) != 1 || calls != 4 {
		t.Fatal(err)
	}
}
func TestSDK_NoCredentialOrDocumentInErrorsOrLogs(t *testing.T) {
	var logs bytes.Buffer
	previous := log.Writer()
	log.SetOutput(&logs)
	defer log.SetOutput(previous)
	secret := "fake-key-never-log"
	document := "fake-document-never-log"
	client := sdkForTest(func(*http.Request) (*http.Response, error) {
		return sdkResponse(401, "application/json", `{"type":"error","error":{"type":"authentication_error","message":"`+secret+document+`"}}`), nil
	})
	_, err := client.Direct(context.Background(), Request{Model: "claude-sonnet-5-5", Effort: "medium", Text: []byte(document)})
	var safe *APIError
	if !errors.As(err, &safe) || safe.Status != 401 || safe.RequestID != "req_test" {
		t.Fatalf("error=%v", err)
	}
	if strings.Contains(err.Error()+logs.String(), secret) || strings.Contains(err.Error()+logs.String(), document) {
		t.Fatal("sensitive content escaped")
	}
}
