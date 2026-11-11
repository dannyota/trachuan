package vbpl

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/sourcegate"
)

func TestClient_SlowRequestLog(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(10 * time.Millisecond)
		_, _ = w.Write([]byte(`{"success":true,"data":{}}`))
	}))
	defer server.Close()
	var log bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&log, nil)))
	defer slog.SetDefault(previous)
	c := testClient(server)
	c.slowThreshold = 5 * time.Millisecond
	ctx := sourcegate.WithItem(context.Background(), "vn:vbpl:1")
	_, _, err := c.Detail(ctx, "1")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"slow source request", "source=vbpl", "endpoint=doc/detail", "item=vn:vbpl:1", "duration=", "outcome=ok"} {
		if !strings.Contains(log.String(), want) {
			t.Errorf("log %q missing %q", log.String(), want)
		}
	}
	if strings.Count(log.String(), "slow source request") != 1 {
		t.Fatalf("log = %q", log.String())
	}
}
