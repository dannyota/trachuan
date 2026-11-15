package testdb

import "testing"

func TestPruneSelection(t *testing.T) {
	const current = "corpus_tpl_0123456789ab"
	tests := []struct {
		name     string
		sessions int
		want     string
	}{
		{"corpus_tpl_abcdef012345", 0, ""},
		{current, 0, "current migration"},
		{"corpus_tpl_abcdef012345", 1, "connected sessions"},
		{"corpus_tpl_ABCDEF012345", 0, "invalid name"},
		{"corpus_tpl_abcdef01234", 0, "invalid name"},
		{"corpus_tpl_abcdef012345x", 0, "invalid name"},
		{"other_corpus_tpl_abcdef012345", 0, "invalid name"},
		{"corpus_tpl_abcdef01234g", 0, "invalid name"},
	}
	for _, tt := range tests {
		t.Run(tt.name+tt.want, func(t *testing.T) {
			if got := pruneSkipReason(tt.name, current, tt.sessions); got != tt.want {
				t.Errorf("skip reason = %q, want %q", got, tt.want)
			}
		})
	}
}
