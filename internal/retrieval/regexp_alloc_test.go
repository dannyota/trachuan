package retrieval

import (
	"strings"
	"testing"
)

func TestParseReferences_Allocations(t *testing.T) {
	allocs := testing.AllocsPerRun(100, func() { _, _ = ParseReferences("no reference") })
	if allocs > 5 {
		t.Fatalf("ParseReferences allocated %.0f times, want at most 5", allocs)
	}
}

func TestValidHash_Allocations(t *testing.T) {
	hash := strings.Repeat("a", 64)
	allocs := testing.AllocsPerRun(100, func() { _ = validHash(hash) })
	if allocs > 2 {
		t.Fatalf("validHash allocated %.0f times, want at most 2", allocs)
	}
}

func TestValidTaxonomyCode_Allocations(t *testing.T) {
	allocs := testing.AllocsPerRun(100, func() { _ = validTaxonomyCode("1.12.c3") })
	if allocs > 2 {
		t.Fatalf("validTaxonomyCode allocated %.0f times, want at most 2", allocs)
	}
}
