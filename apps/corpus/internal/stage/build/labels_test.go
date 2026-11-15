package build

import "testing"

func TestNormalizeLabel_WhitespaceAndNFC(t *testing.T) {
	for _, tc := range []struct{ input, want string }{
		{"\n   Quyết định", "Quyết định"},
		{" Quye\u0302\u0301t\t đi\u0323nh ", "Quyết định"},
		{"  Bộ\u00a0\n Tài   chính ", "Bộ Tài chính"},
		{" \n\t\u2003", ""},
	} {
		if got := normalizeLabel(tc.input); got != tc.want {
			t.Errorf("normalizeLabel(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}
