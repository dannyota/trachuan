package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/ocrcheck"
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

func TestCheck_AbandonOption(t *testing.T) {
	_, err := parseCheckOptions([]string{"--max-usd", "1", "--abandon", "0123456789abcdef0123456789abcdef"}, false)
	if err != nil {
		t.Fatalf("operator abandonment flag rejected: %v", err)
	}
}

func TestCheck_PrintsPerDocumentMetrics(t *testing.T) {
	var out bytes.Buffer
	printCheckDocuments(&out, []ocrcheck.DocumentSummary{
		{EntryID: 12, Pages: 10, InputTokens: 200, OutputTokens: 30, CostMicroUSD: 700, Outcome: "flagged"},
		{EntryID: 13, Pages: 9, InputTokens: 100, OutputTokens: 10, CostMicroUSD: 300, Outcome: "schema:items[2].kind"},
		{EntryID: 14, Outcome: "api_error:invalid_request_error"},
	})
	want := "ocr_text_id=12 pages=10 input_tokens=200 output_tokens=30 cost_usd=0.000700 outcome=flagged\nocr_text_id=13 pages=9 input_tokens=100 output_tokens=10 cost_usd=0.000300 outcome=schema:items[2].kind\nocr_text_id=14 pages=0 input_tokens=0 output_tokens=0 cost_usd=0.000000 outcome=api_error:invalid_request_error\n"
	if out.String() != want {
		t.Fatalf("output=%q", out.String())
	}
}

func TestCheck_RetryFailedOptions(t *testing.T) {
	for _, flags := range [][]string{
		{"--max-usd", "1", "--retry-failed", "--batch", "--limit-docs", "1"},
		{"--max-usd", "1", "--retry-failed", "--dry-run"},
	} {
		if _, err := parseCheckOptions(flags, false); err != nil {
			t.Fatal(err)
		}
	}
	for _, flags := range [][]string{
		{"--max-usd", "1", "--retry-failed", "--reconcile"},
		{"--max-usd", "1", "--retry-failed", "--abandon", "0123456789abcdef0123456789abcdef"},
	} {
		if _, err := parseCheckOptions(flags, false); err == nil {
			t.Fatalf("accepted %v", flags)
		}
	}
}

func TestCheck_EntriesOption(t *testing.T) {
	o, err := parseCheckOptions([]string{"--max-usd", "1", "--entries", "111,328,273"}, false)
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(o)
	if err != nil || !strings.Contains(string(data), `"Entries":[111,328,273]`) {
		t.Fatalf("options=%s err=%v", data, err)
	}
	for _, value := range []string{"", "0", "-1", "1,", "1,1", "abc"} {
		if _, err := parseCheckOptions([]string{"--max-usd", "1", "--entries", value}, false); err == nil {
			t.Fatalf("accepted entries %q", value)
		}
	}
}
