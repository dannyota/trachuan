package eval

import (
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/evaluation"
	"danny.vn/trachuan/internal/retrieval"
)

func TestEvalStage_ReportStatusAndGaps(t *testing.T) {
	r := evaluation.Report{Status: "complete", Floors: []evaluation.Floor{{Metric: evaluation.Metric{Mode: retrieval.Mode("exact"), Name: "recall"}, Status: "fail"}, {Metric: evaluation.Metric{Mode: retrieval.Mode("ann"), Name: "mrr"}, Status: "unavailable"}, {Status: "pass"}}, Rejections: []evaluation.Rejection{{CaseID: "bad", Passed: false}, {CaseID: "ok", Passed: true}}}
	gaps := reportGaps(r)
	if len(gaps) != 3 || gaps[0].Kind != "eval_floor_failed" || gaps[0].Detail != "exact:recall" || gaps[1].Detail != "ann:mrr" || gaps[2].Kind != "eval_rejection_failed" || gaps[2].Detail != "bad" {
		t.Fatalf("gaps: %+v", gaps)
	}
}
