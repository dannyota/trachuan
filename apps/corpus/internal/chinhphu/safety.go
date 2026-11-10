package chinhphu

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
)

func public(ip net.IP) bool {
	if !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() {
		return false
	}
	addr, ok := netip.AddrFromSlice(ip)
	if !ok {
		return false
	}
	addr = addr.Unmap()
	if addr.Is6() && !netip.MustParsePrefix("2000::/3").Contains(addr) {
		return false
	}
	for _, s := range []string{"0.0.0.0/8", "100.64.0.0/10", "192.0.0.0/24", "192.0.2.0/24", "192.88.99.0/24", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "240.0.0.0/4", "2001:db8::/32", "3fff::/20", "2001::/23", "2002::/16"} {
		if netip.MustParsePrefix(s).Contains(addr) {
			return false
		}
	}
	return true
}
func (c *Client) checkURL(ctx context.Context, raw string, file bool) error {
	u, e := url.Parse(raw)
	if e != nil || u.User != nil || u.Hostname() == "" || u.Fragment != "" {
		return fmt.Errorf("invalid source URL: %w", errInvalidPayload)
	}
	origin := u.Scheme + "://" + u.Host
	if c.testHosts[origin] {
		if u.Scheme != "http" && u.Scheme != "https" {
			return fmt.Errorf("invalid test scheme: %w", errInvalidPayload)
		}
		return nil
	}
	host := "vanban.chinhphu.vn"
	if file {
		host = "datafiles.chinhphu.vn"
	}
	if u.Scheme != "https" || !strings.EqualFold(u.Hostname(), host) || (u.Port() != "" && u.Port() != "443") {
		return fmt.Errorf("source URL refused: %w", errInvalidPayload)
	}
	ips, e := c.resolve(ctx, u.Hostname())
	if e != nil {
		return fmt.Errorf("source address unavailable: %w", e)
	}
	if len(ips) == 0 {
		return errors.New("source address unavailable")
	}
	for _, ip := range ips {
		if !public(ip.IP) {
			return fmt.Errorf("source address refused: %w", errInvalidPayload)
		}
	}
	return nil
}
func (c *Client) safeTransport() http.RoundTripper {
	d := &net.Dialer{}
	return &http.Transport{Proxy: nil, ForceAttemptHTTP2: false, TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12}, DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, e := net.SplitHostPort(address)
		if e != nil {
			return nil, errors.New("invalid address")
		}
		for origin := range c.testHosts {
			u, err := url.Parse(origin)
			if err == nil && u.Host == address {
				return d.DialContext(ctx, network, address)
			}
		}
		ips, e := c.resolve(ctx, host)
		if e != nil || len(ips) == 0 {
			return nil, errors.New("address resolution failed")
		}
		for _, ip := range ips {
			if !public(ip.IP) {
				return nil, errors.New("source address refused")
			}
		}
		var last error
		for _, ip := range ips {
			conn, err := d.DialContext(ctx, network, net.JoinHostPort(ip.IP.String(), port))
			if err == nil {
				return conn, nil
			}
			last = err
		}
		return nil, fmt.Errorf("connect source: %w", last)
	}}
}
