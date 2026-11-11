package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestCheck_RequiresSpendBeforeOpeningDatabase(t *testing.T) {
	for _, flags := range [][]string{nil, {"--max-usd", "0"}, {"--max-usd", "NaN"}, {"--max-usd", "1", "--model", "unknown"}} {
		var out, errout bytes.Buffer
		args := append([]string{"ocr", "check", "vn", "20990101"}, flags...)
		code := Run(context.Background(), args, &out, &errout)
		if code != 2 || strings.Contains(errout.String(), "database") {
			t.Fatalf("status=%d error=%s", code, errout.String())
		}
	}
}

func TestCheck_Options(t *testing.T) {
	got, err := parseCheckOptions([]string{"--max-usd", "1", "--batch"}, false)
	if err != nil || got.Model != "claude-sonnet-5-5" || got.Effort != "medium" || !got.Batch {
		t.Fatalf("options=%+v err=%v", got, err)
	}
}
