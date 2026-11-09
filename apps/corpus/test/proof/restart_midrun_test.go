//go:build integration && faultinject

package proof

import (
	"testing"
	"time"
)

func TestProof_RestartMidRun(t *testing.T) {
	t.Parallel()
	baseline := uninterrupted(t)
	h := newHarness(t)
	h.startWorker("")
	h.command("start", "vn", release, "--until", "relate")
	h.waitFetchProgress()
	h.stopWorker()
	time.Sleep(5 * time.Second)
	h.startWorker("")
	h.waitFetchProgressAfterRetry()
	h.waitState("paused")
	h.assertCounts(baseline)
}
