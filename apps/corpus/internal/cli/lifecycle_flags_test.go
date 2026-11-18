package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/run"
)

func TestReportChoice_ReadsStartFlags(t *testing.T) {
	for _, tc := range []struct {
		name string
		opts map[string]string
		want run.ReportChoice
	}{
		{"neither", map[string]string{}, run.ReportChoice{}},
		{"file", map[string]string{"--previous-report": "report.json"}, run.ReportChoice{Path: "report.json"}},
		{"none", map[string]string{"--no-previous-report": "true"}, run.ReportChoice{None: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := reportChoice(tc.opts)
			if err != nil || got != tc.want {
				t.Fatalf("choice=%+v err=%v", got, err)
			}
		})
	}
	for _, opts := range []map[string]string{{"--previous-report": "x", "--no-previous-report": "true"}, {"--previous-report": ""}} {
		if _, err := reportChoice(opts); err == nil {
			t.Fatalf("accepted %v", opts)
		}
	}
}

func TestApplyStartFlags_PrintsDecisionNotice(t *testing.T) {
	var out bytes.Buffer
	var d run.Deps
	applyStartFlags(&d, run.ReportChoice{None: true}, &out)
	if !d.PreviousReport.None || d.Notice == nil {
		t.Fatalf("deps = %+v", d.PreviousReport)
	}
	d.Notice("previous report decision replaced: a -> b")
	if out.String() != "previous report decision replaced: a -> b\n" {
		t.Fatalf("stdout = %q", out.String())
	}
}

func TestStartCLI_RefusesBothFlagsBeforeConnecting(t *testing.T) {
	var out, errout bytes.Buffer
	code := Run(context.Background(), []string{"start", "vn", "20990101", "--previous-report", "r.json", "--no-previous-report"}, &out, &errout)
	if code != 2 || !strings.Contains(errout.String(), "--no-previous-report") {
		t.Fatalf("code=%d err=%s", code, errout.String())
	}
}
