//go:build integration && faultinject

package proof

import "testing"

func TestProof_CrashAfterRename(t *testing.T) {
	t.Parallel()
	proveCrash(t, "after_rename:7")
}
