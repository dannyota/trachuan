//go:build integration && faultinject

package proof

import "testing"

func TestProof_RecoverFromCheckpoint(t *testing.T) {
	t.Parallel()
	baseline := uninterrupted(t)
	h := newHarness(t)
	h.startWorker("")
	h.command("start", "vn", h.rel, "--until", "fetch")
	h.waitState("paused")
	h.stopWorker()
	h.restoreFetch()
	before := h.fetchItems()
	requests := h.gateway.details.Load()
	h.command("recover", "vn", h.rel, "--terminate")
	h.startWorker("")
	h.command("start", "vn", h.rel, "--until", "relate")
	h.waitState("paused")
	after := h.fetchItems()
	if got := h.gateway.details.Load(); got != requests {
		t.Fatalf("fetch requests after recovery = %d, want %d", got, requests)
	}
	if before != after {
		t.Fatalf("fetch items changed after recovery: before %q, after %q", before, after)
	}
	h.assertCounts(baseline)
}
