package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/store"
)

func TestSourceResume_RequiresSourceAndNote(t *testing.T) {
	var out, errout bytes.Buffer
	code := Run(context.Background(), []string{"source-resume", "vn", "20990101"}, &out, &errout)
	if code != 2 || !strings.Contains(errout.String(), "--source") {
		t.Fatalf("code=%d error=%s", code, errout.String())
	}
}

func TestStatus_ShowsPausedSources(t *testing.T) {
	got := pausedSources([]store.RunSource{{Source: "vbpl", State: "active"}, {Source: "chinhphu", State: "source_paused"}})
	if len(got) != 1 || got[0].Source != "chinhphu" {
		t.Fatalf("paused = %+v", got)
	}
}
