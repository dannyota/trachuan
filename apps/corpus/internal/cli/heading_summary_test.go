package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestHeadingSummary_CommandsRejectInvalidArguments(t *testing.T) {
	for _, args := range [][]string{{"heading-scan"}, {"heading-scan", "vn", "20990101", "--unknown"}, {"heading-exclusions"}, {"heading-reviews"}} {
		var out, errout bytes.Buffer
		if code := Run(context.Background(), args, &out, &errout); code != 2 {
			t.Fatalf("%v: code %d: %s", args, code, &errout)
		}
		if !strings.Contains(errout.String(), "usage: corpus "+args[0]) {
			t.Fatalf("command missing: %s", &errout)
		}
	}
}
