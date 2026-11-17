package loadrun

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPreflight_RequiredQueries(t *testing.T) {
	c := Config{Phases: []string{"A", "B", "C", "D"}, DurationA: 10 * time.Minute, DurationC: 5 * time.Minute, DurationD: 5 * time.Minute}
	if got := requiredQueries(c, 3200); got != 3400 {
		t.Fatalf("required %d", got)
	}
}

func TestPreflight_RoundsEachPhase(t *testing.T) {
	c := Config{Phases: []string{"A", "C", "D"}, DurationA: time.Millisecond, DurationC: time.Millisecond, DurationD: time.Millisecond}
	if got := requiredQueries(c, 0); got != 3 {
		t.Fatalf("required %d", got)
	}
}

func TestPreflight_OnlyFetchesSummaryList(t *testing.T) {
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.URL.Path)
		_, _ = fmt.Fprint(w, `{"items":[{"key":"vn:fixture:1"}]}`)
	}))
	defer server.Close()
	err := Run(context.Background(), Config{Pages: server.URL, API: server.URL, PublicURL: server.URL, Out: filepath.Join(t.TempDir(), "out"), Queries: []string{"only one"}, Phases: []string{"A"}, DurationA: time.Second})
	if err == nil || !strings.Contains(err.Error(), "insufficient queries") {
		t.Fatalf("error %v", err)
	}
	if len(calls) != 1 || calls[0] != "/api/v1/public/documents" {
		t.Fatalf("calls %v", calls)
	}
}
