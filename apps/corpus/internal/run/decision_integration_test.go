//go:build integration

package run

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/stage"
)

func storedDecision(t *testing.T, d Deps) (*stage.PreviousReport, string) {
	t.Helper()
	var raw []byte
	var message *string
	if err := d.DB.QueryRow(context.Background(), `SELECT snapshot, message FROM run`).Scan(&raw, &message); err != nil {
		t.Fatal(err)
	}
	var snapshot stage.Snapshot
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		t.Fatal(err)
	}
	if message == nil {
		return snapshot.PreviousReport, ""
	}
	return snapshot.PreviousReport, *message
}

func TestStart_NewRunNeedsDecisionAndWritesNoRow(t *testing.T) {
	t.Parallel()
	d := lifecycleDeps(t)
	if err := Start(context.Background(), d, "vn", d.TaskQueue, "", ""); err == nil || !strings.Contains(err.Error(), "--previous-report") {
		t.Fatalf("err = %v", err)
	}
	var count int
	if err := d.DB.QueryRow(context.Background(), `SELECT count(*) FROM run`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("runs=%d err=%v", count, err)
	}
}

func TestStart_NewRunFreezesDecision(t *testing.T) {
	t.Parallel()
	d := lifecycleDeps(t)
	d.PreviousReport = ReportChoice{None: true}
	if err := Start(context.Background(), d, "vn", d.TaskQueue, "", ""); err != nil {
		t.Fatal(err)
	}
	decision, _ := storedDecision(t, d)
	if decision == nil || !decision.None() {
		t.Fatalf("decision = %+v", decision)
	}
}

func TestStart_ReplacesDecisionUntilEvalItemExistsAndPrintsBoth(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	d := lifecycleDeps(t)
	var notices []string
	d.Notice = func(message string) { notices = append(notices, message) }
	snapshot := testSnapshot()
	frozen := strings.Repeat("a", 64)
	snapshot.PreviousReport = &stage.PreviousReport{SHA256: frozen, Path: "previous-report/x"}
	next := "fetch"
	insertLifecycleRun(t, d, "paused", time.Now(), snapshot, &next)
	if _, err := d.DB.Exec(ctx, `INSERT INTO run_item(run_id,stage,item_key,state,batch) SELECT id,'eval','vn/x','pending',0 FROM run`); err != nil {
		t.Fatal(err)
	}
	d.PreviousReport = ReportChoice{None: true}
	if err := Start(ctx, d, "vn", d.TaskQueue, "", ""); err == nil || !strings.Contains(err.Error(), "eval item") {
		t.Fatalf("change after eval item: %v", err)
	}
	if decision, _ := storedDecision(t, d); decision == nil || decision.SHA256 != frozen {
		t.Fatalf("decision = %+v", decision)
	}
	if len(notices) != 0 {
		t.Fatalf("refused start printed %v", notices)
	}
	if _, err := d.DB.Exec(ctx, `DELETE FROM run_item WHERE stage='eval'`); err != nil {
		t.Fatal(err)
	}
	if err := Start(ctx, d, "vn", d.TaskQueue, "", ""); err != nil {
		t.Fatal(err)
	}
	decision, message := storedDecision(t, d)
	if decision == nil || !decision.None() || message != "" {
		t.Fatalf("decision=%+v message=%q", decision, message)
	}
	if len(notices) != 1 || !strings.Contains(notices[0], frozen) || !strings.Contains(notices[0], stage.NoPreviousReport) {
		t.Fatalf("notices = %v", notices)
	}
}

func runWithoutDecision() stage.Snapshot {
	snapshot := testSnapshot()
	snapshot.PreviousReport = nil
	return snapshot
}

func TestStart_MissingDecisionAboveCutoffRefusedBeforeGenerationBump(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	d := lifecycleDeps(t)
	next := "fetch"
	insertLifecycleRun(t, d, "paused", time.Now(), runWithoutDecision(), &next)
	if err := Start(ctx, d, "vn", d.TaskQueue, "", ""); err == nil || !strings.Contains(err.Error(), "--previous-report") {
		t.Fatalf("err = %v", err)
	}
	var generation int
	var state string
	if err := d.DB.QueryRow(ctx, `SELECT generation, state FROM run`).Scan(&generation, &state); err != nil || generation != 1 || state != "paused" {
		t.Fatalf("generation=%d state=%s err=%v", generation, state, err)
	}
}

func TestStart_FlagAddsDecisionAboveCutoffBeforeEval(t *testing.T) {
	t.Parallel()
	d := lifecycleDeps(t)
	next := "fetch"
	insertLifecycleRun(t, d, "paused", time.Now(), runWithoutDecision(), &next)
	d.PreviousReport = ReportChoice{None: true}
	if err := Start(context.Background(), d, "vn", d.TaskQueue, "", ""); err != nil {
		t.Fatal(err)
	}
	if decision, _ := storedDecision(t, d); decision == nil || !decision.None() {
		t.Fatalf("decision = %+v", decision)
	}
}

func TestStart_FlagAboveCutoffRefusedOnceEvalItemExists(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	d := lifecycleDeps(t)
	next := "fetch"
	insertLifecycleRun(t, d, "paused", time.Now(), runWithoutDecision(), &next)
	if _, err := d.DB.Exec(ctx, `INSERT INTO run_item(run_id,stage,item_key,state,batch) SELECT id,'eval','vn/x','pending',0 FROM run`); err != nil {
		t.Fatal(err)
	}
	d.PreviousReport = ReportChoice{None: true}
	if err := Start(ctx, d, "vn", d.TaskQueue, "", ""); err == nil || !strings.Contains(err.Error(), "eval item") {
		t.Fatalf("err = %v", err)
	}
}
