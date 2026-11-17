package cli

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"time"

	"danny.vn/trachuan/internal/embedding"
)

func localFrameworkEmbedderClient(address string) (*embedding.Client, error) {
	u, err := url.Parse(address)
	if err != nil || u.Scheme != "http" || u.User != nil || u.Host == "" || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return nil, fmt.Errorf("invalid local embedder URL")
	}
	host := u.Hostname()
	ip, ipErr := netip.ParseAddr(host)
	if host != "localhost" && (ipErr != nil || !ip.IsLoopback()) {
		return nil, fmt.Errorf("embedder URL must use loopback")
	}
	port := u.Port()
	if port == "" {
		port = "80"
	}
	if host == "localhost" {
		host = "127.0.0.1"
	}
	target := net.JoinHostPort(host, port)
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "tcp", target)
	}}
	hc := &http.Client{Transport: transport, Timeout: 125 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return embedding.NewClient(address, hc), nil
}
