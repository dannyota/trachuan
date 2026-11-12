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
	"strings"
	"syscall"
	"testing"

	"github.com/anthropics/anthropic-sdk-go/v2"
	"golang.org/x/net/http2"
)

func fields(t *testing.T, v any) map[string]any {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestDiagnostics_ErrorClasses(t *testing.T) {
	cases := []struct {
		name string
		err  error
	}{
		{"http_status", &anthropic.Error{}}, {"timeout", context.DeadlineExceeded},
		{"canceled", context.Canceled}, {"eof", io.EOF}, {"eof", io.ErrUnexpectedEOF},
		{"connection_reset", syscall.ECONNRESET}, {"broken_pipe", syscall.EPIPE},
		{"http2_stream", http2.StreamError{StreamID: 1, Code: http2.ErrCodeCancel}},
		{"http2_stream", &http2.StreamError{StreamID: 1, Code: http2.ErrCodeCancel}},
		{"http2_stream", http2.GoAwayError{}}, //nolint:staticcheck // The transport may still return this type.
		{"tls", &tls.RecordHeaderError{}}, {"tls", tls.RecordHeaderError{}},
		{"tls", tls.AlertError(42)}, {"tls", &tls.CertificateVerificationError{}},
		{"dns", &net.DNSError{Err: "sentinel-private-message"}}, {"other", errors.New("sentinel-private-message")},
	}
	for i, tc := range cases {
		t.Run(fmt.Sprintf("%s_%d", tc.name, i), func(t *testing.T) {
			err := fmt.Errorf("outer sentinel-private-message: %w", fmt.Errorf("inner: %w", tc.err))
			got := sanitizeSDKError(err)
			m := fields(t, got)
			if m["Class"] != tc.name {
				t.Errorf("class=%v want=%s", m["Class"], tc.name)
			}
			if strings.Contains(fmt.Sprint(m, got), "sentinel-private-message") {
				t.Error("error message escaped")
			}
		})
	}
}

func TestDiagnostics_InnermostType(t *testing.T) {
	err := fmt.Errorf("outer: %w", fmt.Errorf("inner: %w", &net.OpError{Op: "sentinel-private-message"}))
	m := fields(t, sanitizeSDKError(err))
	if m["Type"] != "*net.OpError" {
		t.Errorf("type=%v", m["Type"])
	}
	if strings.Contains(fmt.Sprint(m), "sentinel-private-message") {
		t.Fatal("error message escaped")
	}
}

type errorReader struct{ io.Reader }

func (r errorReader) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	if err == io.EOF {
		return n, errors.New("sentinel-private-message")
	}
	return n, err
}
func TestDiagnostics_StreamEvents(t *testing.T) {
	body := strings.Split(streamMessage(true), "event: content_block_stop")[0]
	client := sdkForTest(func(*http.Request) (*http.Response, error) {
		r := sdkResponse(200, "text/event-stream", "")
		r.Body = io.NopCloser(errorReader{strings.NewReader(body)})
		return r, nil
	})
	response, err := client.Direct(context.Background(), Request{Model: "claude-sonnet-5-5", Effort: "medium"})
	if err == nil {
		t.Fatal("expected stream error")
	}
	m := fields(t, response.Usage)
	if m["response_events"] != float64(3) {
		t.Errorf("events=%v want=3", m["response_events"])
	}
	if strings.Contains(fmt.Sprint(m, err), "sentinel-private-message") {
		t.Fatal("error message escaped")
	}
}

func TestDiagnostics_RequestBytes(t *testing.T) {
	r, repo, _, docs, _ := jpegFixture(t)
	if err := run(t, r, docs, jpegOptions()); err != nil {
		t.Fatal(err)
	}
	if len(repo.attempts) != 1 {
		t.Fatalf("attempts=%d", len(repo.attempts))
	}
	var u Usage
	for _, a := range repo.attempts {
		u = a.Usage
	}
	got := fields(t, u)["request_bytes"]
	p, reason := r.prepare(context.Background(), docs[0], jpegOptions())
	if reason != "" {
		t.Fatal(reason)
	}
	size, err := jpegRequestSize(p.request)
	if err != nil {
		t.Fatal(err)
	}
	if got != float64(size) {
		t.Errorf("request bytes=%v want=%d", got, size)
	}
}

func TestDiagnostics_BatchUncertainRecordsErrorAndBodySize(t *testing.T) {
	r, repo, client, docs := fixture(t)
	client.CreateError = fmt.Errorf("outer: %w", fmt.Errorf("inner: %w", syscall.ECONNRESET))
	o := options()
	o.Batch = true
	if err := run(t, r, docs, o); err == nil {
		t.Fatal("expected uncertain submission")
	}
	if len(repo.attempts) != 1 {
		t.Fatalf("attempts=%d", len(repo.attempts))
	}
	a := repo.attempts[0]
	m := fields(t, a.Usage)
	if m["error_class"] != "connection_reset" {
		t.Errorf("class=%v", m["error_class"])
	}
	if m["error_type"] != "syscall.Errno" {
		t.Errorf("type=%v", m["error_type"])
	}
	p, reason := r.prepare(context.Background(), docs[0], o)
	if reason != "" {
		t.Fatal(reason)
	}
	p.request.CustomID = customID(a)
	data, err := json.Marshal(anthropic.MessageBatchNewParams{Requests: []anthropic.MessageBatchNewParamsRequest{batchRequest(p.request)}})
	if err != nil {
		t.Fatal(err)
	}
	if m["request_bytes"] != float64(len(data)) {
		t.Errorf("request bytes=%v want=%d", m["request_bytes"], len(data))
	}
}

func TestDiagnostics_CountsPingBeforeStreamError(t *testing.T) {
	body := "event: ping\ndata: {}\n\n" + strings.Split(streamMessage(true), "event: content_block_stop")[0]
	client := sdkForTest(func(*http.Request) (*http.Response, error) {
		r := sdkResponse(200, "text/event-stream", "")
		r.Body = io.NopCloser(errorReader{strings.NewReader(body)})
		return r, nil
	})
	response, err := client.Direct(context.Background(), Request{Model: "claude-sonnet-5-5", Effort: "medium"})
	if err == nil {
		t.Fatal("expected stream error")
	}
	if got := fields(t, response.Usage)["response_events"]; got != float64(4) {
		t.Errorf("events=%v want=4", got)
	}
}

func TestDiagnostics_IncompleteStream(t *testing.T) {
	client := sdkForTest(func(*http.Request) (*http.Response, error) {
		return sdkResponse(200, "text/event-stream", streamMessage(false)), nil
	})
	_, err := client.Direct(context.Background(), Request{Model: "claude-sonnet-5-5", Effort: "medium"})
	if err == nil || fields(t, err)["Class"] != "incomplete_stream" {
		t.Fatalf("error=%v", err)
	}
}

func TestDiagnostics_UncertainAttemptKeepsStreamDiagnostics(t *testing.T) {
	r, repo, c, docs := concurrentFixture(t, 1)
	body := strings.Split(streamMessage(true), "event: content_block_stop")[0]
	sdk := sdkForTest(func(*http.Request) (*http.Response, error) {
		response := sdkResponse(200, "text/event-stream", "")
		response.Body = io.NopCloser(errorReader{strings.NewReader(body)})
		return response, nil
	})
	var bodyBytes int
	c.call = func(ctx context.Context, req Request) (Response, error) {
		encoded, err := json.Marshal(messageParams(req))
		if err != nil {
			t.Fatal(err)
		}
		bodyBytes = len(encoded) + len(`,"stream":true`)
		return sdk.Direct(ctx, req)
	}
	if err := run(t, r, docs, jpegOptions()); err == nil {
		t.Fatal("expected uncertain submission")
	}
	if len(repo.attempts) != 1 {
		t.Fatalf("attempts=%d", len(repo.attempts))
	}
	a := repo.attempts[0]
	m := fields(t, a.Usage)
	if a.State != "uncertain" || m["response_events"] != float64(3) || m["request_bytes"] != float64(bodyBytes) || m["error_class"] != "other" {
		t.Errorf("state=%s usage=%v", a.State, m)
	}
	if strings.Contains(fmt.Sprint(m), "sentinel-private-message") {
		t.Fatal("error message escaped")
	}
}

func TestDiagnostics_PreparationFailureKeepsPreparedSize(t *testing.T) {
	r, repo, _, docs := fixture(t)
	p := prepared{doc: docs[0], requestBytes: 12345, pages: 10}
	if err := r.failPrepared(context.Background(), p, options(), "over_check_limit"); err != nil {
		t.Fatal(err)
	}
	if got := repo.attempts[0].Usage.RequestBytes; got != p.requestBytes {
		t.Fatalf("request bytes=%d want=%d", got, p.requestBytes)
	}
}

type failedCountClient struct{ Client }

func (c failedCountClient) CountTokens(context.Context, Request) (int64, error) {
	return 0, errors.New("count failed")
}

func TestDiagnostics_CountFailureKeepsPreparedSize(t *testing.T) {
	for _, concurrent := range []bool{false, true} {
		t.Run(fmt.Sprint(concurrent), func(t *testing.T) {
			var r *Runner
			var repo *memoryRepository
			var docs []Document
			var o Options
			if concurrent {
				r, repo, _, docs = concurrentFixture(t, 1)
				o = jpegOptions()
			} else {
				r, repo, _, docs = fixture(t)
				o = options()
			}
			p, reason := r.prepare(context.Background(), docs[0], o)
			if reason != "" {
				t.Fatal(reason)
			}
			if p.requestBytes == 0 {
				t.Fatal("request size not prepared")
			}
			r.Client = failedCountClient{Client: r.Client}
			if err := run(t, r, docs, o); err != nil {
				t.Fatal(err)
			}
			if got := repo.attempts[0].Usage.RequestBytes; got != p.requestBytes {
				t.Fatalf("request bytes=%d want=%d", got, p.requestBytes)
			}
		})
	}
}

func TestDiagnostics_ContextLimitKeepsPreparedSize(t *testing.T) {
	r, repo, c, docs := concurrentFixture(t, 1)
	o := jpegOptions()
	p, reason := r.prepare(context.Background(), docs[0], o)
	if reason != "" {
		t.Fatal(reason)
	}
	c.tokens = 1_000_000
	if err := run(t, r, docs, o); err != nil {
		t.Fatal(err)
	}
	if repo.attempts[0].Reason != "over_check_limit" {
		t.Fatalf("reason=%s", repo.attempts[0].Reason)
	}
	if got := repo.attempts[0].Usage.RequestBytes; got != p.requestBytes {
		t.Fatalf("request bytes=%d want=%d", got, p.requestBytes)
	}
}
