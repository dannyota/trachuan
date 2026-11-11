package oauth

import "testing"

func TestRedirect_LoopbackPortOnly(t *testing.T) {
	for _, host := range []string{"127.0.0.1", "[::1]", "localhost"} {
		base := "http://" + host
		for _, tc := range []struct {
			registered, requested string
			want                  bool
		}{{base + ":123/cb?x=1", base + ":456/cb?x=1", true}, {base + "/cb", base + ":456/cb", true}, {base + "/cb", base + ":456/other", false}, {base + "/cb?x=1", base + ":456/cb?x=2", false}} {
			if got := MatchRedirect(tc.registered, tc.requested); got != tc.want {
				t.Errorf("%s %s = %v", tc.registered, tc.requested, got)
			}
		}
	}
}
func TestRedirect_ExactOtherwise(t *testing.T) {
	for _, tc := range []struct {
		a, b string
		want bool
	}{{"https://example.test/cb", "https://example.test/cb", true}, {"https://example.test:1/cb", "https://example.test:2/cb", false}, {"http://example.test/cb", "http://example.test/cb", false}, {"https://example.test/cb", "https://example.test/cb?x=1", false}} {
		if got := MatchRedirect(tc.a, tc.b); got != tc.want {
			t.Errorf("%s %s = %v", tc.a, tc.b, got)
		}
	}
}
