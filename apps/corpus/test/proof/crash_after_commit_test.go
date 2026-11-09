//go:build integration && faultinject

package proof

import "testing"

func TestProof_CrashAfterCommit(t *testing.T) {
	t.Parallel()
	proveCrash(t, "after_commit:7")
}
