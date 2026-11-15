package store

import "testing"

func TestParseFrameworkScope_Shape(t *testing.T) {
	for _, tc := range []struct {
		raw   string
		valid bool
	}{
		{"*", true}, {"fw-alpha", true}, {"fw-beta2", true}, {"alpha", true},
		{"", false}, {"FW-alpha", false}, {"fw--alpha", false}, {"-fw", false}, {"fw-", false},
		{"fw_alpha", false}, {"fw.alpha", false}, {"fw-*", false}, {" fw-alpha", false}, {"fw-alpha\n", false},
		{"1fw", false}, {"fw-é", false},
	} {
		t.Run(tc.raw, func(t *testing.T) {
			got, err := ParseFrameworkScope(tc.raw)
			if (err == nil) != tc.valid || err == nil && got != tc.raw {
				t.Fatalf("scope=%q error=%v", got, err)
			}
		})
	}
}
