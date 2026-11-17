package observe

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/cmd/loadcheck/internal/loadrun"
)

func TestReport_MissingBaselineIsUnavailable(t *testing.T) {
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	requests := []loadrun.Record{{Phase: "C", Stream: "search", Status: 200, Start: at, DurationMS: 10}, {Phase: "C", Stream: "reader", Status: 200, Start: at, DurationMS: 10}}
	server := `{"msg":"reader pool wait","time":"2026-01-01T00:00:00Z","bucket_bounds_ms":[50],"bucket_counts":[1,0],"count":1}`
	gates, err := buildReport(requests, loadrun.Report{}, nil, strings.NewReader(""), strings.NewReader(server))
	if err != nil {
		t.Fatal(err)
	}
	for _, g := range gates {
		if g.Name == "C search" {
			if g.Result != "unavailable" {
				t.Fatalf("result %s", g.Result)
			}
			return
		}
	}
	t.Fatal("missing gate")
}

func TestReport_CacheHitsUsePhaseWindow(t *testing.T) {
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	records := []loadrun.Record{{Phase: "A", Stream: "search", Status: 200, Start: at, DurationMS: 10}, {Phase: "A", Stream: "reader", Status: 200, Start: at, DurationMS: 10}}
	var web strings.Builder
	for i := 0; i < 500; i++ {
		key := fmt.Sprintf("vn:fixture:%d", i)
		records = append(records, loadrun.Record{Phase: "C", Stream: "summary", Key: key, Pass: 1, Status: 200, Start: at.Add(time.Second), DurationMS: 10}, loadrun.Record{Phase: "C", Stream: "summary", Key: key, Pass: 2, Status: 200, Start: at.Add(2 * time.Second), DurationMS: 10})
		if i < 6 {
			_, _ = fmt.Fprintf(&web, "{\"msg\":\"summary render\",\"time\":\"2026-01-01T00:00:01.500Z\",\"path\":\"/vn/documents/%s/summary\"}\n{\"msg\":\"summary render\",\"time\":\"2026-01-01T00:00:01.600Z\",\"path\":\"/vn/documents/%s/summary\"}\n", key, key)
		}
	}
	server := `{"msg":"reader pool wait","time":"2026-01-01T00:00:02Z","bucket_bounds_ms":[50],"bucket_counts":[1,0],"count":1}`
	gates, err := buildReport(records, loadrun.Report{}, nil, strings.NewReader(web.String()), strings.NewReader(server))
	if err != nil {
		t.Fatal(err)
	}
	for _, g := range gates {
		if g.Name == "C hit rate" {
			if g.Result != "fail" || !strings.Contains(g.Value, "494/500") {
				t.Fatalf("gate %+v", g)
			}
			return
		}
	}
	t.Fatal("missing hit gate")
}
