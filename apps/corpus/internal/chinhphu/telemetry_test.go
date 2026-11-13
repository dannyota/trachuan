package chinhphu

import (
	"context"
	"net/http"
	"testing"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/sourcegate"
)

func TestTelemetry_HomeRetries(t *testing.T) {
	calls := 0
	c, base := clientFor(t, func(w http.ResponseWriter, _ *http.Request) {
		calls++
		if calls == 1 {
			w.WriteHeader(500)
			return
		}
		_, _ = w.Write([]byte("<html></html>"))
	}, WithBackoffs(time.Second), WithClock(time.Now, func(context.Context, time.Duration) error { return nil }))
	ctx, telemetry := sourcegate.WithTelemetry(context.Background())
	ctx = sourcegate.WithEndpoint(ctx, "home")
	if _, _, err := c.fetchPage(ctx, http.MethodGet, base, nil); err != nil {
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
		if row.Source != "chinhphu" || row.Endpoint != "home" || row.Attempt != i+1 || row.Status == nil || *row.Status != wantStatus || row.Outcome != wantOutcome {
			t.Errorf("row=%+v", row)
		}
	}
}

func TestTelemetry_InvalidBody(t *testing.T) {
	c, base := clientFor(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = w.Write([]byte("invalid"))
	})
	ctx, telemetry := sourcegate.WithTelemetry(context.Background())
	ctx = sourcegate.WithEndpoint(ctx, "home")
	if _, _, err := c.fetchPage(ctx, http.MethodGet, base, nil); err == nil {
		t.Fatal("invalid media type accepted")
	}
	rows := telemetry.Records()
	if len(rows) != 1 || rows[0].Outcome != "invalid_body" || rows[0].Status == nil || *rows[0].Status != 200 {
		t.Fatalf("rows=%+v", rows)
	}
}

func TestTelemetry_InvalidDetailFields(t *testing.T) {
	c, _ := clientFor(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html><body>unexpected</body></html>"))
	})
	ctx, telemetry := sourcegate.WithTelemetry(context.Background())
	if _, _, err := c.Detail(ctx, 1); err == nil {
		t.Fatal("invalid detail accepted")
	}
	rows := telemetry.Records()
	if len(rows) != 1 || rows[0].Outcome != "invalid_body" {
		t.Fatalf("rows=%+v", rows)
	}
}

func TestTelemetry_InvalidSearchPage(t *testing.T) {
	c, _ := clientFor(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html><body>unexpected</body></html>"))
	})
	ctx, telemetry := sourcegate.WithTelemetry(context.Background())
	if _, _, err := c.Search(ctx, "query-sentinel"); err != nil {
		t.Fatal(err)
	}
	rows := telemetry.Records()
	if len(rows) != 2 || rows[0].Endpoint != "home" || rows[0].Outcome != "ok" || rows[1].Endpoint != "search" || rows[1].Outcome != "invalid_body" {
		t.Fatalf("rows=%+v", rows)
	}
}
