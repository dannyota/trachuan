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

func TestRedirect_InvalidOnEitherSide(t *testing.T) {
	const valid = "http://localhost/cb"
	for _, tc := range []struct{ name, uri string }{
		{"fragment", "http://localhost/cb#fragment"},
		{"userinfo", "http://user@localhost/cb"},
		{"malformed IPv6", "http://[::1/cb"},
		{"non-loopback HTTP", "http://example.test/cb"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if MatchRedirect(tc.uri, valid) || MatchRedirect(valid, tc.uri) || MatchRedirect(tc.uri, tc.uri) {
				t.Fatal("accepted invalid redirect")
			}
		})
	}
}

func TestRedirect_ByteDifferences(t *testing.T) {
	for _, tc := range []struct{ name, registered, requested string }{
		{"host case", "http://localhost:123/cb", "http://LOCALHOST:456/cb"},
		{"percent encoding", "http://localhost:123/%63b", "http://localhost:456/cb"},
		{"trailing slash", "http://localhost:123/cb", "http://localhost:456/cb/"},
		{"query order", "http://localhost:123/cb?a=1&b=2", "http://localhost:456/cb?b=2&a=1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if MatchRedirect(tc.registered, tc.requested) || MatchRedirect(tc.requested, tc.registered) {
				t.Fatal("accepted different redirect")
			}
		})
	}
}
