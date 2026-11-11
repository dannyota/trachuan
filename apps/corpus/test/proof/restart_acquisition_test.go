//go:build integration && faultinject

package proof

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/run"
	"danny.vn/trachuan/apps/corpus/internal/source"
	"danny.vn/trachuan/apps/corpus/internal/sourcegate"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/stage/discover"
	"danny.vn/trachuan/apps/corpus/internal/stage/expand"
	"danny.vn/trachuan/apps/corpus/internal/stage/fetch"
	"danny.vn/trachuan/apps/corpus/internal/vbpl"
	"danny.vn/trachuan/apps/corpus/internal/worker"
	"danny.vn/trachuan/apps/corpus/internal/workflow"
	"go.temporal.io/sdk/client"
)

func TestProof_RestartReusesAcquisitionsWithRepeatingExpandGap(t *testing.T) {
	h := newHarness(t)
	ctx, cancel := context.WithTimeout(context.Background(), 55*time.Second)
	defer cancel()
	h.ctx = ctx
	h.gateway.unavailableDiagram = "12"
	// Inject short source retries without changing the worker's frozen recipes.
	vb := vbpl.NewWithBase(h.gateway.server.URL, "proof", vbpl.WithTestHTTP(), vbpl.WithLimits(1000, 16), vbpl.WithFileHosts("127.0.0.1"), vbpl.WithBackoffs(), vbpl.WithGate(sourcegate.New(h.db)))
	registry := workflow.NewRegistry(discover.New(vb), expand.New(vb), fetch.New(vb))
	temporal, err := client.Dial(client.Options{HostPort: h.server.FrontendHostPort(), Namespace: h.namespace})
	if err != nil {
		t.Fatal(err)
	}
	defer temporal.Close()
	w, err := worker.New(ctx, worker.Deps{Env: stage.Env{DB: h.db, Artifacts: artifact.Store{Root: h.artifacts}}, Temporal: temporal, Registry: registry, TaskQueue: h.rel, HeartbeatEvery: 100 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	workerCtx, stop := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- w.Run(workerCtx) }()
	defer func() {
		stop()
		if e := <-done; e != nil {
			t.Error(e)
		}
	}()
	list, err := source.Parse([]byte("country: vn\nvbpl:\n  selection_version: 1\n  sweep_agencies: ['62']\nrelation_codes:\n  '10': {kind: amends_supplements, label: amends, amending: true}\nstatus_codes: {}\n"))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(list)
	if err != nil {
		t.Fatal(err)
	}
	d := run.Deps{DB: h.db, Temporal: temporal, TaskQueue: h.rel, Stages: registry.Names(), RecipesFor: registry.Recipes, SourceList: raw}
	for generation := 1; generation <= 3; generation++ {
		before := h.gateway.details.Load()
		if err = run.Start(ctx, d, "vn", h.rel, "fetch", ""); err != nil {
			t.Fatal(err)
		}
		h.waitState("paused")
		// Wait for completion after the final MarkRun transaction.
		if err = temporal.GetWorkflow(ctx, run.WorkflowID("vn", h.rel), "").Get(ctx, nil); err != nil {
			t.Fatal(err)
		}
		if generation > 1 && h.gateway.details.Load() != before+2 {
			t.Fatalf("restart detail requests=%d, want 2", h.gateway.details.Load()-before)
		}
		if n := h.gateway.attachments.Load(); n != 11 {
			t.Fatalf("attachment requests=%d, want 11", n)
		}
		report, statusErr := run.Status(ctx, d, "vn", h.rel)
		if statusErr != nil {
			t.Fatal(statusErr)
		}
		want := 0
		if generation > 1 {
			want = 11
		}
		if report.AdoptedItems != want {
			t.Fatalf("adopted=%d, want %d", report.AdoptedItems, want)
		}
		var gaps int
		if err = h.db.QueryRow(ctx, `SELECT count(*) FROM gap WHERE stage='expand' AND kind='source_unavailable'`).Scan(&gaps); err != nil || gaps != 1 {
			t.Fatalf("expand gaps=%d: %v", gaps, err)
		}
	}
}
