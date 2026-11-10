package retrieval

import "testing"

func TestNormalize_CyrillicLookalikes(t *testing.T) {
	for _, tc := range []struct{ input, want string }{
		{"280/2026/NĐ-CР", "280/2026/ND-CP"},
		{"АВЕКМНОРСТХ", "ABEKMHOPCTX"},
	} {
		if got := NormalizeNumber(tc.input); got != tc.want {
			t.Errorf("NormalizeNumber(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestNormalize_ExistingCasesUnchanged(t *testing.T) {
	if got := NormalizeNumber("  71 / 2025 / qh15  "); got != "71/2025/QH15" {
		t.Errorf("got %q", got)
	}
}
