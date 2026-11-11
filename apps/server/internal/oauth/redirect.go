package oauth

import (
	"net/url"
	"strings"
)

// MatchRedirect validates and compares redirect URIs, ignoring only loopback ports.
func MatchRedirect(registered, requested string) bool {
	a, ok := redirectURI(registered)
	if !ok {
		return false
	}
	b, ok := redirectURI(requested)
	if !ok {
		return false
	}
	if a.Scheme != "http" || b.Scheme != "http" {
		return registered == requested
	}
	return withoutPort(registered, a) == withoutPort(requested, b)
}
func redirectURI(raw string) (*url.URL, bool) {
	if len(raw) == 0 || len(raw) > 512 {
		return nil, false
	}
	for i := range raw {
		if raw[i] < 0x21 || raw[i] > 0x7e || raw[i] == '#' {
			return nil, false
		}
	}
	u, err := url.Parse(raw)
	if err != nil || u.Opaque != "" || u.Host == "" || u.Hostname() == "" || u.User != nil {
		return nil, false
	}
	if u.Scheme == "https" {
		return u, true
	}
	if u.Scheme == "http" && (strings.EqualFold(u.Hostname(), "localhost") || u.Hostname() == "127.0.0.1" || u.Hostname() == "::1") {
		return u, true
	}
	return nil, false
}
func withoutPort(raw string, u *url.URL) string {
	if u.Port() == "" {
		return raw
	}
	start := len(u.Scheme) + 3
	return raw[:start] + strings.TrimSuffix(u.Host, ":"+u.Port()) + raw[start+len(u.Host):]
}
