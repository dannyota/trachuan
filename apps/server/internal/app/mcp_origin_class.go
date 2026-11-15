package app

import (
	"net"
	"net/netip"
	"net/url"
	"strconv"
	"strings"

	"golang.org/x/net/idna"
)

func mcpOriginClass(values []string) string {
	if len(values) > 1 {
		return "multiple"
	}
	if len(values) != 1 || !validMCPOrigin(values[0]) {
		return "malformed"
	}
	u, err := url.Parse(values[0])
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return "malformed"
	}
	host := strings.ToLower(u.Hostname())
	if ip, parseErr := netip.ParseAddr(host); parseErr == nil {
		if ip.Zone() != "" {
			return "malformed"
		}
		host = ip.String()
	} else {
		host, err = idna.Lookup.ToASCII(host)
		if err != nil {
			return "malformed"
		}
	}
	port := u.Port()
	if port != "" {
		n, _ := strconv.Atoi(port)
		port = strconv.Itoa(n)
	}
	if (u.Scheme == "https" && port == "443") || (u.Scheme == "http" && port == "80") {
		port = ""
	}
	if port != "" {
		host = net.JoinHostPort(host, port)
	} else if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	canonical := u.Scheme + "://" + host
	return canonical[:min(len(canonical), 200)]
}
