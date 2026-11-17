package observe

import (
	"strings"
	"testing"
	"time"
)

func TestRestarts_Identity(t *testing.T) {
	tracker := newTracker()
	if tracker.observe("web", []process{{PID: 1, Start: "a"}}) {
		t.Fatal("initial process restarted")
	}
	if tracker.observe("web", []process{{PID: 1, Start: "a"}}) {
		t.Fatal("unchanged process restarted")
	}
	if !tracker.observe("web", []process{{PID: 1, Start: "b"}}) {
		t.Fatal("start change missed")
	}
	if !tracker.observe("web", []process{{PID: 2, Start: "b"}}) {
		t.Fatal("PID change missed")
	}
}
func TestRenders_WindowAndValidation(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(10 * time.Second)
	input := `{"msg":"summary render","time":"2026-01-01T00:00:02.000Z","path":"/vn/documents/vn:fixture:1/summary"}` + "\n" + `{"msg":"summary render","time":"2026-01-01T00:00:04.000Z","path":"/vn/documents/vn:fixture:1/summary"}` + "\n" + `{"msg":"other"}`
	counts, err := renders(strings.NewReader(input), start, end)
	if err != nil || counts["vn:fixture:1"] != 2 {
		t.Fatalf("%v %v", counts, err)
	}
	if _, err = renders(strings.NewReader(`{"msg":"summary render","path":"/x"}`), start, end); err == nil {
		t.Fatal("missing timestamp accepted")
	}
	if _, err = renders(strings.NewReader(`{"msg":"summary render","time":"2026-01-01T07:00:02+07:00","path":"/vn/documents/vn:fixture:1/summary"}`), start, end); err == nil {
		t.Fatal("non-UTC timestamp accepted")
	}
}
func TestBuckets_CombinedP99(t *testing.T) {
	logs := `{"msg":"reader pool wait","time":"2026-01-01T00:00:01Z","bucket_bounds_ms":[1,50],"bucket_counts":[99,1,0],"count":100}` + "\n" + `{"msg":"reader pool wait","time":"2026-01-01T00:00:02Z","bucket_bounds_ms":[1,50],"bucket_counts":[0,0,1],"count":1}`
	got, err := poolP99(strings.NewReader(logs), time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 1, 1, 0, 1, 0, 0, time.UTC))
	if err != nil || got != 50 {
		t.Fatalf("p99 %v err %v", got, err)
	}
}
func TestLatency_Relative(t *testing.T) {
	if !latencyGate(125, 200, 100, 1000, 2000) || latencyGate(126, 200, 100, 1000, 2000) {
		t.Fatal("relative gate")
	}
}
