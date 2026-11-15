package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"
)

func originRequest(mux http.Handler, origin, method, path string) {
	r := httptest.NewRequest(method, path, nil)
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	mux.ServeHTTP(httptest.NewRecorder(), r)
}

func originLogs(t *testing.T, logs *bytes.Buffer) []map[string]any {
	t.Helper()
	var records []map[string]any
	for _, line := range bytes.Split(bytes.TrimSpace(logs.Bytes()), []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		var record map[string]any
		if err := json.Unmarshal(line, &record); err != nil {
			t.Fatal(err)
		}
		if record["level"] != "INFO" {
			t.Fatalf("wrong level: %v", record)
		}
		records = append(records, record)
	}
	return records
}

func assertOriginSummary(t *testing.T, record map[string]any, total, distinct, overflow float64) {
	t.Helper()
	if record["total"] != total || record["distinct"] != distinct || record["overflow"] != overflow {
		t.Fatalf("summary=%v want total=%v distinct=%v overflow=%v", record, total, distinct, overflow)
	}
}

func TestMCPOriginTracker_DedupeAndHourlySummary(t *testing.T) {
	for _, mode := range []string{"log", "enforce"} {
		t.Run(mode, func(t *testing.T) {
			now := time.Date(2099, 1, 1, 10, 59, 0, 0, time.UTC)
			var logs bytes.Buffer
			mux, _ := originRouteFixture(t, mode, nil, func() time.Time { return now }, slog.New(slog.NewJSONHandler(&logs, nil)))
			originRequest(mux, "https://outside.test", "GET", "/mcp?secret=fixture")
			originRequest(mux, "https://outside.test", "POST", "/oauth/token")
			originRequest(mux, "https://second.test", "GET", "/mcp")
			records := originLogs(t, &logs)
			if len(records) != 2 {
				t.Fatalf("got %d log lines, want 2", len(records))
			}
			if records[0]["origin"] != "https://outside.test" || records[0]["path"] != "/mcp" || records[0]["mode"] != mode || len(records[0]) != 6 || strings.Contains(logs.String(), "secret") {
				t.Fatalf("unexpected observation fields: %v", records[0])
			}
			now = now.Add(time.Minute)
			originRequest(mux, "https://example.test", "GET", "/mcp")
			records = originLogs(t, &logs)
			if len(records) != 3 {
				t.Fatalf("hour rollover logged %d lines, want 3", len(records))
			}
			assertOriginSummary(t, records[2], 3, 2, 0)
			originRequest(mux, "https://outside.test", "GET", "/mcp")
			if len(originLogs(t, &logs)) != 4 {
				t.Fatal("origin was not logged again in the next hour")
			}
			now = now.Add(3 * time.Hour)
			originRequest(mux, "", "GET", "/mcp")
			records = originLogs(t, &logs)
			if len(records) != 5 {
				t.Fatal("rollover emitted summaries for empty hours")
			}
			assertOriginSummary(t, records[4], 1, 1, 0)
			now = now.Add(time.Hour)
			originRequest(mux, "", "GET", "/mcp")
			if len(originLogs(t, &logs)) != 5 {
				t.Fatal("empty hour emitted a summary")
			}
		})
	}
}

func TestMCPOriginTracker_BoundedOverflow(t *testing.T) {
	now := time.Date(2099, 1, 1, 10, 0, 0, 0, time.UTC)
	var logs bytes.Buffer
	mux, _ := originRouteFixture(t, "enforce", nil, func() time.Time { return now }, slog.New(slog.NewJSONHandler(&logs, nil)))
	for i := range 100 {
		originRequest(mux, fmt.Sprintf("https://outside%d.test", i), "GET", "/mcp")
	}
	for range 2 {
		originRequest(mux, "https://overflow.test", "GET", "/mcp")
	}
	originRequest(mux, "https://outside0.test", "GET", "/mcp")
	if len(originLogs(t, &logs)) != 100 {
		t.Fatal("tracker logged beyond the 100-origin bound")
	}
	now = now.Add(time.Hour)
	originRequest(mux, "https://overflow.test", "GET", "/mcp")
	records := originLogs(t, &logs)
	if len(records) != 102 {
		t.Fatalf("got %d lines, want 102", len(records))
	}
	assertOriginSummary(t, records[100], 103, 100, 2)
	if records[101]["origin"] != "https://overflow.test" {
		t.Fatal("tracker did not admit an overflow origin in the new hour")
	}
}

func TestMCPOriginTracker_TruncatesCanonicalClass(t *testing.T) {
	var logs bytes.Buffer
	policy := newMCPOriginPolicy("https://example.test", nil, "log", slog.New(slog.NewJSONHandler(&logs, nil)), time.Now)
	handler := policy.middleware("", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) }))
	origin := "https://" + strings.Repeat(strings.Repeat("a", 50)+".", 4) + "test"
	originRequest(handler, origin, "GET", "/mcp")
	records := originLogs(t, &logs)
	if len(records) != 1 {
		t.Fatal("missing origin class")
	}
	logged, ok := records[0]["origin"].(string)
	if !ok || len(logged) != 200 || !utf8.ValidString(logged) || !strings.HasPrefix(origin, logged) {
		t.Fatal("canonical origin class was not bounded")
	}
	if _, stored := policy.tracker.origins[logged]; !stored || len(policy.tracker.origins) != 1 {
		t.Fatal("tracker retained a value other than the logged class")
	}
}

func TestMCPOriginTracker_OnlyStoresClasses(t *testing.T) {
	for _, mode := range []string{"log", "enforce"} {
		t.Run(mode, func(t *testing.T) {
			var logs bytes.Buffer
			policy := newMCPOriginPolicy("https://example.test", nil, mode, slog.New(slog.NewJSONHandler(&logs, nil)), time.Now)
			handler := policy.middleware("", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) }))
			for _, values := range [][]string{
				{"https://synthetic-user:synthetic-password@outside.test"},
				{"https://outside.test?synthetic-query=fixture"},
				{"https://outside.test#synthetic-fragment"},
				{"https://outside.test/synthetic-path"},
				{strings.Repeat("synthetic-junk", 1000)},
				{"https://outside.test", "synthetic-second-header"},
				{"synthetic-first-header", "https://outside.test"},
				{"https://outside.test, synthetic-folded-header"},
				{"null"}, {""},
				{"HTTPS://OUTSIDE.TEST:443"}, {"https://outside.test"},
			} {
				r := httptest.NewRequest("GET", "/mcp", nil)
				r.Header["Origin"] = values
				handler.ServeHTTP(httptest.NewRecorder(), r)
			}
			if strings.Contains(logs.String(), "synthetic-") {
				t.Fatal("synthetic sensitive input reached the log")
			}
			for key := range policy.tracker.origins {
				if key != "malformed" && key != "multiple" && key != "https://outside.test" {
					t.Fatal("tracker retained raw input instead of an origin class")
				}
			}
			if len(policy.tracker.origins) != 3 || len(originLogs(t, &logs)) != 3 || policy.tracker.total != 12 {
				t.Fatal("tracker did not deduplicate classes or count all outside requests")
			}
		})
	}
}

func TestMCPOriginTracker_ConcurrentRequests(t *testing.T) {
	now := time.Date(2099, 1, 1, 10, 0, 0, 0, time.UTC)
	var logs bytes.Buffer
	mux, _ := originRouteFixture(t, "enforce", nil, func() time.Time { return now }, slog.New(slog.NewJSONHandler(&logs, nil)))
	var wg sync.WaitGroup
	for range 500 {
		wg.Go(func() { originRequest(mux, "https://outside.test", "GET", "/mcp") })
	}
	wg.Wait()
	now = now.Add(time.Hour)
	originRequest(mux, "", "GET", "/mcp")
	records := originLogs(t, &logs)
	if len(records) != 2 {
		t.Fatalf("got %d lines, want 2", len(records))
	}
	assertOriginSummary(t, records[1], 500, 1, 0)
}
