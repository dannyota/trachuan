//go:build integration && faultinject

package proof

import "testing"

func TestProof_RestartMidRun(t *testing.T) {
	t.Parallel()
	baseline := uninterrupted(t)
	h := newHarness(t)
	h.gateway.blockAfterFirst = make(chan struct{})
	h.startWorker("")
	h.command("start", "vn", h.rel, "--until", "relate")
	h.waitFetchProgress()
	h.stopWorker()
	close(h.gateway.blockAfterFirst)
	h.startWorker("")
	h.waitFetchProgressAfterRetry()
	h.waitState("paused")
	h.assertCounts(baseline)
}
