//go:build integration

package run_test

import (
	"context"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/run"
)

func TestTreeTextPartialGap_StatusAndAcceptance(t *testing.T) {
	f := newAcquisitionFixture(t)
	f.exec(`DELETE FROM run_item WHERE run_id=$1`, f.run.ID)
	f.exec(`INSERT INTO run_item(run_id,stage,item_key,state,batch) VALUES($1,'extract','vn:vbpl:1','done',0)`, f.run.ID)
	f.exec(`INSERT INTO gap(run_id,stage,item_key,kind,detail) VALUES($1,'extract','vn:vbpl:1','tree_text_partial','tree_chars=100 transcription_chars=300')`, f.run.ID)
	ctx := context.Background()
	report, err := run.Status(ctx, f.deps(), "vn", f.run.Rel)
	if err != nil || report.Stages["extract"].UnacceptedGaps["tree_text_partial"] != 1 {
		t.Fatalf("stages=%+v err=%v", report.Stages, err)
	}
	gaps, err := run.ListGaps(ctx, f.deps(), "vn", f.run.Rel)
	if err != nil || len(gaps) != 1 || gaps[0].Kind != "tree_text_partial" || gaps[0].Detail != "tree_chars=100 transcription_chars=300" {
		t.Fatalf("gaps=%+v err=%v", gaps, err)
	}
	eligible, _, err := run.Publishable(ctx, f.env.DB, f.run.ID)
	if err != nil || eligible {
		t.Fatalf("publishable=%v err=%v", eligible, err)
	}
	n, err := run.AcceptGaps(ctx, f.deps(), "vn", f.run.Rel, "tree_text_partial", "reviewed missing article text")
	if err != nil || n != 1 {
		t.Fatalf("accepted=%d err=%v", n, err)
	}
	eligible, reasons, err := run.Publishable(ctx, f.env.DB, f.run.ID)
	if err != nil || !eligible {
		t.Fatalf("publishable=%v reasons=%v err=%v", eligible, reasons, err)
	}
	report, err = run.Status(ctx, f.deps(), "vn", f.run.Rel)
	if err != nil || report.Stages["extract"].UnacceptedGaps["tree_text_partial"] != 0 {
		t.Fatalf("stages=%+v err=%v", report.Stages, err)
	}
}
