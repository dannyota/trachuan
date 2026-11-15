package release

import (
	"bytes"
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
