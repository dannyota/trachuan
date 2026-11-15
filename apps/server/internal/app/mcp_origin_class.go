package app

import (
	"net/netip"
	"net/url"
	"strconv"
	"strings"
)

func mcpOriginClass(values []string) string {
	if len(values) > 1 {
		return "multiple"
	}
	if len(values) != 1 || !validMCPOrigin(values[0]) {
		return "malformed"
	}
	return values[0]
}

func validMCPOrigin(raw string) bool {
	if len(raw) == 0 || len(raw) > 200 {
		return false
	}
	for i := range len(raw) {
		if raw[i] <= ' ' || raw[i] >= 0x7f {
			return false
		}
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.Hostname() == "" || u.User != nil || u.Opaque != "" || u.Path != "" || u.RawPath != "" || strings.ContainsAny(raw, "?#,") || strings.HasSuffix(u.Host, ":") {
		return false
	}
	if strings.HasPrefix(u.Host, "[") || strings.Contains(u.Hostname(), ":") {
		ip, parseErr := netip.ParseAddr(u.Hostname())
		if parseErr != nil || !ip.Is6() || ip.Zone() != "" || !strings.HasPrefix(u.Host, "[") {
			return false
		}
	}
	if port := u.Port(); port != "" {
		n, parseErr := strconv.Atoi(port)
		if parseErr != nil || n < 1 || n > 65535 {
			return false
		}
	}
	return true
}
