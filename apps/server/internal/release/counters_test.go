package release

import (
	"bytes"
	"io"
	"log/slog"
	"strings"
	"testing"
)

func TestResolver_RetirementLogsCountersOnce(t *testing.T) {
	var out bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&out, nil)))
	defer slog.SetDefault(previous)
	r, _, _ := resolverFixture(t)
	takeLease(t, r, "").Done()
	r.Close()
	r.Close()
	got := out.String()
	if strings.Count(got, `"msg":"retrieval counters"`) != 1 || !strings.Contains(got, `"release":"20990103"`) || !strings.Contains(got, `"fallback_groups":0`) || !strings.Contains(got, `"lexical_sql":0`) {
		t.Fatalf("log=%s", got)
	}
}

func TestResolver_CounterLoggingDuringResolves(t *testing.T) {
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	defer slog.SetDefault(previous)
	r, _, _ := resolverFixture(t)
	lease := takeLease(t, r, "")
	engine := lease.Engine
	lease.Done()
	r.mu.Lock()
	entry := r.entries["corpus_vn_20990103"]
	r.mu.Unlock()
	start := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		<-start
		for range 100 {
			entry.logCounters(engine)
		}
	}()
	close(start)
	for range 100 {
		takeLease(t, r, "").Done()
	}
	<-done
	r.Close()
}
