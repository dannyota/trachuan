package build

import "testing"

func TestDatabaseName_ValidatesIdentity(t *testing.T) {
	for _, tc := range []struct {
		country, rel, want string
	}{
		{"vn", "20261109", "corpus_vn_20261109"},
		{"vn", "20261109_12", "corpus_vn_20261109_12"},
		{"VN", "20261109", ""},
		{"vn; DROP DATABASE x", "20261109", ""},
		{"vn", "20261109_123", ""},
		{"vn", "20261109;DROP", ""},
	} {
		got, err := DatabaseName(tc.country, tc.rel)
		if tc.want == "" {
			if err == nil {
				t.Errorf("DatabaseName(%q, %q) accepted %q", tc.country, tc.rel, got)
			}
		} else if err != nil || got != tc.want {
			t.Errorf("DatabaseName(%q, %q) = %q, %v; want %q", tc.country, tc.rel, got, err, tc.want)
		}
	}
}
