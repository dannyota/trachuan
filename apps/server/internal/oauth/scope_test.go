package oauth

import "testing"

func TestScope_DefaultAndInvalid(t *testing.T) {
	for _, tc := range []struct {
		in, want string
		valid    bool
	}{{"", "mcp:read", true}, {"mcp:read", "mcp:read", true}, {"other", "", false}, {"mcp:read mcp:read", "", false}, {" mcp:read", "", false}} {
		got, err := ParseScope(tc.in)
		if got != tc.want || (err == nil) != tc.valid {
			t.Errorf("scope %q = %q, %v", tc.in, got, err)
		}
	}
}
