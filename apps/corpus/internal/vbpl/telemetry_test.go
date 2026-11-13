package vbpl

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/sourcegate"
	"golang.org/x/time/rate"
)

func TestTelemetry_DiagramRetry(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var calls atomic.Int32
		transport := deadlineRoundTrip(func(r *http.Request) (*http.Response, error) {
			status := 200
			if calls.Add(1) == 1 {
				status = 500
			}
			return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"success":true,"data":{}}`)), Request: r}, nil
		})
		client := NewWithBase("https://example.test", "test", WithTransport(transport), WithLimits(float64(rate.Inf), 8), WithBackoffs(time.Second))
		ctx, telemetry := sourcegate.WithTelemetry(context.Background())
		if _, err := client.Diagram(ctx, "identifier-sentinel"); err != nil {
			t.Fatal(err)
		}
		rows := telemetry.Records()
		if len(rows) != 2 {
			t.Fatalf("rows=%+v", rows)
		}
		for i, row := range rows {
			wantStatus, wantOutcome := 500, "http_error"
			if i == 1 {
				wantStatus, wantOutcome = 200, "ok"
			}
			if row.Source != "vbpl" || row.Endpoint != "diagram" || row.Attempt != i+1 || row.Status == nil || *row.Status != wantStatus || row.Outcome != wantOutcome {
				t.Errorf("row=%+v", row)
			}
		}
	})
}

func TestTelemetry_NonJSONRetry(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var calls atomic.Int32
		transport := deadlineRoundTrip(func(r *http.Request) (*http.Response, error) {
			body := `{"success":true,"data":{}}`
			if calls.Add(1) == 1 {
				body = "not json"
			}
			return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
		})
		client := NewWithBase("https://example.test", "test", WithTransport(transport), WithLimits(float64(rate.Inf), 8), WithBackoffs(time.Second))
		ctx, telemetry := sourcegate.WithTelemetry(context.Background())
		if _, err := client.Diagram(ctx, "identifier-sentinel"); err != nil {
			t.Fatal(err)
		}
		rows := telemetry.Records()
		if len(rows) != 2 || rows[0].Outcome != "invalid_body" || rows[0].Status == nil || *rows[0].Status != 200 || rows[1].Outcome != "ok" {
			t.Fatalf("rows=%+v", rows)
		}
	})
}

func TestTelemetry_InvalidDetailFields(t *testing.T) {
	transport := deadlineRoundTrip(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"success":true,"data":{"id":42}}`)), Request: r}, nil
	})
	client := NewWithBase("https://example.test", "test", WithTransport(transport), WithLimits(float64(rate.Inf), 8))
	ctx, telemetry := sourcegate.WithTelemetry(context.Background())
	if _, _, err := client.Detail(ctx, "sentinel"); err == nil {
		t.Fatal("invalid detail accepted")
	}
	rows := telemetry.Records()
	if len(rows) != 1 || rows[0].Outcome != "invalid_body" {
		t.Fatalf("rows=%+v", rows)
	}
}

func TestTelemetry_InvalidListingFields(t *testing.T) {
	transport := deadlineRoundTrip(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"success":true,"data":{"items":false}}`)), Request: r}, nil
	})
	client := NewWithBase("https://example.test", "test", WithTransport(transport), WithLimits(float64(rate.Inf), 8))
	ctx, telemetry := sourcegate.WithTelemetry(context.Background())
	if _, err := client.List(ctx, ListQuery{Page: 1}); err == nil {
		t.Fatal("invalid listing accepted")
	}
	rows := telemetry.Records()
	if len(rows) != 1 || rows[0].Endpoint != "list" || rows[0].Outcome != "invalid_body" {
		t.Fatalf("rows=%+v", rows)
	}
}

func TestTelemetry_UnexpectedSuccessStatus(t *testing.T) {
	transport := deadlineRoundTrip(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 204, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("")), Request: r}, nil
	})
	client := NewWithBase("https://example.test", "test", WithTransport(transport), WithLimits(float64(rate.Inf), 8))
	ctx, telemetry := sourcegate.WithTelemetry(context.Background())
	if _, err := client.Diagram(ctx, "sentinel"); err == nil {
		t.Fatal("unexpected status accepted")
	}
	rows := telemetry.Records()
	if len(rows) != 1 || rows[0].Status == nil || *rows[0].Status != 204 || rows[0].Outcome != "http_error" {
		t.Fatalf("rows=%+v", rows)
	}
}

type bareTimeout struct{}

func (bareTimeout) Error() string   { return "timeout" }
func (bareTimeout) Timeout() bool   { return true }
func (bareTimeout) Temporary() bool { return true }

func TestTelemetry_TransportTimeoutWithoutDeadline(t *testing.T) {
	transport := deadlineRoundTrip(func(*http.Request) (*http.Response, error) { return nil, bareTimeout{} })
	client := NewWithBase("https://example.test", "test", WithTransport(transport), WithLimits(float64(rate.Inf), 8), WithBackoffs())
	ctx, telemetry := sourcegate.WithTelemetry(context.Background())
	_, err := client.Diagram(ctx, "sentinel")
	var failure *Failure
	if !errors.As(err, &failure) || failure.Cause != "timeout" {
		t.Fatalf("error=%v", err)
	}
	rows := telemetry.Records()
	if len(rows) != 1 || rows[0].Outcome != "timeout" || rows[0].Status != nil {
		t.Fatalf("rows=%+v", rows)
	}
}
