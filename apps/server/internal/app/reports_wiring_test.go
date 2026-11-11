package app

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/store"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type routeTransport struct{ handler http.Handler }

func (tr routeTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer trc_"+strings.Repeat("A", 43))
	w := httptest.NewRecorder()
	tr.handler.ServeHTTP(w, r)
	return w.Result(), nil
}

func TestRoutes_ReportsMounted(t *testing.T) {
	mux, _, _ := routeFixture(t)
	for _, tt := range []struct{ method, path string }{{"POST", "/api/v1/reports"}, {"GET", "/api/v1/me/reports"}, {"DELETE", "/api/v1/me/reports/1"}} {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest(tt.method, tt.path, nil))
		if w.Code != http.StatusUnauthorized {
			t.Errorf("%s %s: status %d", tt.method, tt.path, w.Code)
		}
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	session, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{Endpoint: "http://localhost/mcp", HTTPClient: &http.Client{Transport: routeTransport{mux}}, DisableStandaloneSSE: true, MaxRetries: -1}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = session.Close() }()
	list, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range list.Tools {
		if tool.Name == "report_issue" {
			return
		}
	}
	t.Fatal("report_issue missing from tools/list")
}

type reportPurgeStore struct {
	failingPurgeStore
	reports   chan store.DeleteExpiredReportsParams
	snapshots chan store.DeleteExpiredSnapshotsParams
}

func (s *reportPurgeStore) DeleteExpiredReports(_ context.Context, p store.DeleteExpiredReportsParams) (int64, error) {
	s.reports <- p
	return 0, nil
}
func (s *reportPurgeStore) DeleteExpiredSnapshots(_ context.Context, p store.DeleteExpiredSnapshotsParams) (int64, error) {
	s.snapshots <- p
	return 0, nil
}
func TestPurge_DeletesClosedReports(t *testing.T) {
	now := time.Date(2026, 11, 11, 0, 0, 0, 0, time.UTC)
	s := &reportPurgeStore{failingPurgeStore: failingPurgeStore{done: make(chan struct{}, 1)}, reports: make(chan store.DeleteExpiredReportsParams, 1), snapshots: make(chan store.DeleteExpiredSnapshotsParams, 1)}
	ticks := make(chan time.Time, 1)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		runPurgeLoop(ctx, s, func() time.Time { return now }, slog.New(slog.NewTextHandler(io.Discard, nil)), ticks)
	}()
	ticks <- now
	defer func() { cancel(); <-done }()
	select {
	case p := <-s.reports:
		if !p.Before.Equal(now.Add(-90*24*time.Hour)) || p.BatchLimit != 1000 {
			t.Errorf("report retention: %+v", p)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("report retention did not run")
	}
	select {
	case p := <-s.snapshots:
		if !p.Before.Equal(now.Add(-90*24*time.Hour)) || p.BatchLimit != 1000 {
			t.Errorf("snapshot retention: %+v", p)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("snapshot retention did not run")
	}
}
