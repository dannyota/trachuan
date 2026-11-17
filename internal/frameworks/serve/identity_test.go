package serve

import (
	"slices"
	"testing"
)

func TestIdentity_PublicReleaseAndFrameworkCodes(t *testing.T) {
	e := testEngine()
	if e.Release() != e.release {
		t.Fatal("release identity differs")
	}
	codes := e.FrameworkCodes()
	if !slices.Equal(codes, []string{"fw-alpha", "fw-beta"}) {
		t.Fatalf("codes=%v", codes)
	}
	codes[0] = "changed"
	if e.FrameworkCodes()[0] != "fw-alpha" {
		t.Fatal("caller mutated engine identity")
	}
}
