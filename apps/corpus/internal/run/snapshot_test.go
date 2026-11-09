package run

import (
	"testing"

	"danny.vn/traquydinh/apps/corpus/internal/stage"
)

func TestValidateRel(t *testing.T) {
	for _, tc := range []struct {
		rel   string
		valid bool
	}{
		{"20261110", true},
		{"20261110_2", true},
		{"2026-11-10", false},
		{"20261110_100", false},
	} {
		if got := validateRel(tc.rel) == nil; got != tc.valid {
			t.Errorf("validateRel(%q) valid = %v, want %v", tc.rel, got, tc.valid)
		}
	}
}

func TestCompareRel(t *testing.T) {
	if CompareRel("20261110_10", "20261110_2") <= 0 {
		t.Fatal("sequence 10 must follow sequence 2")
	}
}

func TestSnapshot_Extend(t *testing.T) {
	original := stage.Snapshot{Stages: []string{"discover"}, Recipes: map[string]string{"discover": "one"}}
	updated, changed, err := extendSnapshot(original, []string{"discover", "fetch"}, map[string]string{"discover": "one", "fetch": "two"})
	if err != nil || !changed || len(updated.Stages) != 2 || updated.Recipes["fetch"] != "two" {
		t.Fatalf("extend = %+v, %v, %v", updated, changed, err)
	}
	if len(original.Stages) != 1 || len(original.Recipes) != 1 {
		t.Fatal("extension changed the frozen input")
	}
	if _, _, err = extendSnapshot(original, []string{"discover"}, map[string]string{"discover": "changed"}); err == nil {
		t.Fatal("changed frozen recipe accepted")
	}
}
