package testpg

import (
	"regexp"
	"testing"
)

func TestReleaseCandidates_ValidAndDeterministic(t *testing.T) {
	a := releaseCandidates("example")
	b := releaseCandidates("example")
	if len(a) != 36500 || len(b) != len(a) {
		t.Fatalf("candidate count = %d", len(a))
	}
	pattern := regexp.MustCompile(`^2099[0-9]{4}(_[0-9]{1,2})?$`)
	seen := make(map[string]bool)
	for i, rel := range a {
		if !pattern.MatchString(rel) || seen[rel] || rel != b[i] {
			t.Fatalf("invalid, repeated or unstable candidate %d: %q", i, rel)
		}
		seen[rel] = true
	}
	if a[0] != "20990424" || a[1] != a[0]+"_1" || a[99] != a[0]+"_99" || a[100] != "20990425" {
		t.Fatalf("unexpected order: %q %q %q %q", a[0], a[1], a[99], a[100])
	}
}

func TestDropCorpusDatabase_RunKeyForms(t *testing.T) {
	for _, tc := range []struct {
		key   string
		valid bool
	}{
		{"vn/20990101", true}, {"vn/20991231_1", true}, {"vn/20990228_99", true}, {"vn/20990229_1", false}, {"vn/20260101", false}, {"vn/20990101_", false}, {"vn/20990101_123", false}, {"vn/20990101_a", false},
	} {
		if got := validRunKey(tc.key); got != tc.valid {
			t.Errorf("validRunKey(%q) = %v", tc.key, got)
		}
	}
}
