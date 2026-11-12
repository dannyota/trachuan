package retrieval

import (
	"strings"
	"testing"
)

func TestValidHash_Format(t *testing.T) {
	valid := strings.Repeat("0123456789abcdef", 4)
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"lowercase hex", valid, true},
		{"63 bytes", valid[:63], false},
		{"65 bytes", valid + "0", false},
		{"uppercase hex", "A" + valid[1:], false},
		{"non-hex first", "g" + valid[1:], false},
		{"non-hex middle", valid[:32] + "g" + valid[33:], false},
		{"non-hex last", valid[:63] + "g", false},
		{"empty", "", false},
		{"multibyte rune", "é" + valid[2:], false},
		{"leading space", " " + valid, false},
		{"trailing space", valid + " ", false},
		{"leading newline", "\n" + valid, false},
		{"trailing newline", valid + "\n", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := validHash(tt.input); got != tt.want {
				t.Errorf("validHash(%q) = %t, want %t", tt.input, got, tt.want)
			}
		})
	}
}
