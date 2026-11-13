package build

import (
	"sync/atomic"
	"testing"
)

func TestIdentityPool_StopDrainsScheduledJobs(t *testing.T) {
	for i := 0; i < 100; i++ {
		started := make(chan struct{})
		release := make(chan struct{})
		var second atomic.Bool
		pool := newIdentityPool(2, 1, func(index int) identityResult {
			if index == 0 {
				close(started)
				<-release
			} else {
				second.Store(true)
			}
			return identityResult{}
		})
		<-started
		pool.jobs <- 1
		stopped := make(chan struct{})
		go func() { pool.stop(); close(stopped) }()
		<-pool.done
		close(release)
		<-stopped
		if !second.Load() {
			t.Fatal("scheduled item was abandoned")
		}
	}
}
