package oauth

import (
	"container/list"
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

const cimdRefusedRanges = `
0.0.0.0/8 10.0.0.0/8 100.64.0.0/10 127.0.0.0/8 169.254.0.0/16
172.16.0.0/12 192.0.0.0/24 192.0.2.0/24 192.31.196.0/24 192.52.193.0/24
192.88.99.0/24 192.168.0.0/16 192.175.48.0/24 198.18.0.0/15 198.51.100.0/24
203.0.113.0/24 224.0.0.0/4 240.0.0.0/4
::/128 ::1/128 64:ff9b::/96 64:ff9b:1::/48 100::/64 100:0:0:1::/64
2001::/23 2001:db8::/32 2002::/16 2620:4f:8000::/48 3fff::/20 5f00::/16
fc00::/7 fe80::/10 fec0::/10 ff00::/8`

type cimdEntry struct {
	client  Client
	expires time.Time
}
type cimdFlight struct {
	done   chan struct{}
	client Client
	err    error
}

// CIMDResolver owns a bounded cache and at most four simultaneous fetches.
// Its dependencies are fixed at construction and never changed during use.
type CIMDResolver struct {
	roots   *x509.CertPool
	lookup  func(context.Context, string) ([]netip.Addr, error)
	dial    func(context.Context, string, string) (net.Conn, error)
	clock   func() time.Time
	timeout time.Duration
	refused []netip.Prefix
	mu      sync.Mutex
	cache   map[string]*list.Element
	lru     list.List
	flights map[string]*cimdFlight
}

// NewCIMDResolver uses system DNS and TLS trust without proxies or redirects.
func NewCIMDResolver() *CIMDResolver {
	f := &CIMDResolver{
		clock: time.Now, timeout: 5 * time.Second,
		lookup: func(ctx context.Context, host string) ([]netip.Addr, error) {
			return net.DefaultResolver.LookupNetIP(ctx, "ip", host)
		},
		cache: map[string]*list.Element{}, flights: map[string]*cimdFlight{},
	}
	for _, raw := range strings.Fields(cimdRefusedRanges) {
		f.refused = append(f.refused, netip.MustParsePrefix(raw))
	}
	return f
}

func cloneClient(c Client) Client { c.RedirectURIs = slices.Clone(c.RedirectURIs); return c }

// Resolve validates a DNS URL, fetches its document and caches successful
// metadata for 24 hours. Distinct misses beyond the fetch cap fail immediately.
func (f *CIMDResolver) Resolve(ctx context.Context, id string) (Client, error) {
	if err := ctx.Err(); err != nil {
		return Client{}, fmt.Errorf("resolve client: %w", err)
	}
	u, err := cimdURL(id)
	if err != nil {
		return Client{}, err
	}
	if _, parseErr := netip.ParseAddr(u.Hostname()); parseErr == nil {
		return Client{}, ErrInvalidClient
	}
	id = strings.Clone(id)
	f.mu.Lock()
	if element := f.cache[id]; element != nil {
		entry := element.Value.(cimdEntry)
		if f.clock().Before(entry.expires) {
			f.lru.MoveToFront(element)
			f.mu.Unlock()
			return cloneClient(entry.client), nil
		}
		delete(f.cache, id)
		f.lru.Remove(element)
	}
	if flight := f.flights[id]; flight != nil {
		f.mu.Unlock()
		select {
		case <-ctx.Done():
			return Client{}, fmt.Errorf("wait for client metadata: %w", ctx.Err())
		case <-flight.done:
			return cloneClient(flight.client), flight.err
		}
	}
	if len(f.flights) >= 4 {
		f.mu.Unlock()
		return Client{}, ErrTemporarilyUnavailable
	}
	flight := &cimdFlight{done: make(chan struct{})}
	f.flights[id] = flight
	f.mu.Unlock()
	fetchCtx, cancel := context.WithTimeout(ctx, f.timeout)
	c, err := f.fetch(fetchCtx, id, u)
	cancel()
	f.mu.Lock()
	if err == nil {
		for len(f.cache) >= 1000 {
			old := f.lru.Back()
			delete(f.cache, old.Value.(cimdEntry).client.ID)
			f.lru.Remove(old)
		}
		f.cache[id] = f.lru.PushFront(cimdEntry{cloneClient(c), f.clock().Add(24 * time.Hour)})
	}
	flight.client, flight.err = c, err
	delete(f.flights, id)
	close(flight.done)
	f.mu.Unlock()
	return cloneClient(c), err
}

func (f *CIMDResolver) addressAllowed(addr netip.Addr) bool {
	if !addr.IsValid() || addr.Is4In6() || addr.Zone() != "" || !addr.IsGlobalUnicast() || addr.IsPrivate() {
		return false
	}
	if addr.Is6() && !netip.MustParsePrefix("2000::/3").Contains(addr) {
		return false
	}
	for _, prefix := range f.refused {
		if prefix.Contains(addr) {
			return false
		}
	}
	return true
}

func (f *CIMDResolver) checkDial(target string, port uint16, approved []netip.Addr) error {
	literal, err := netip.ParseAddrPort(target)
	if err != nil || literal.Port() != port || !f.addressAllowed(literal.Addr()) || !slices.Contains(approved, literal.Addr()) {
		return ErrInvalidClient
	}
	return nil
}

func (f *CIMDResolver) connect(ctx context.Context, target string, port uint16, approved []netip.Addr) (net.Conn, error) {
	if err := f.checkDial(target, port, approved); err != nil {
		return nil, err
	}
	if f.dial != nil {
		return f.dial(ctx, "tcp", target)
	}
	dialer := net.Dialer{ControlContext: func(_ context.Context, _ string, literal string, _ syscall.RawConn) error {
		return f.checkDial(literal, port, approved)
	}}
	return dialer.DialContext(ctx, "tcp", target)
}

func (f *CIMDResolver) fetch(ctx context.Context, id string, u *url.URL) (Client, error) {
	resolved, err := f.lookup(ctx, u.Hostname())
	if err != nil || len(resolved) == 0 {
		return Client{}, ErrInvalidClient
	}
	addresses := slices.Clone(resolved)
	for _, addr := range addresses {
		if !f.addressAllowed(addr) {
			return Client{}, ErrInvalidClient
		}
	}
	port := uint16(443)
	if u.Port() != "" {
		n, _ := strconv.ParseUint(u.Port(), 10, 16)
		port = uint16(n)
	}
	target := netip.AddrPortFrom(addresses[0], port).String()
	transport := &http.Transport{
		Proxy: nil, DisableCompression: true, DisableKeepAlives: true, MaxResponseHeaderBytes: 8192,
		DialTLSContext: func(_ context.Context, _ string, address string) (net.Conn, error) {
			if address != target {
				return nil, ErrInvalidClient
			}
			// The transport detaches its dial context from the request deadline.
			// Use the complete fetch context for dialing and the TLS handshake.
			for _, addr := range addresses {
				conn, connectErr := f.connect(ctx, netip.AddrPortFrom(addr, port).String(), port, addresses)
				if connectErr != nil {
					continue
				}
				tlsConn := tls.Client(conn, &tls.Config{MinVersion: tls.VersionTLS12, ServerName: u.Hostname(), RootCAs: f.roots})
				if handshakeErr := tlsConn.HandshakeContext(ctx); handshakeErr != nil {
					_ = conn.Close()
					continue
				}
				return tlsConn, nil
			}
			return nil, ErrInvalidClient
		},
	}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	pinned := "https://" + target + id[len("https://")+len(u.Host):]
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pinned, nil)
	if err != nil {
		return Client{}, ErrInvalidClient
	}
	req.Host = u.Host
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "trachuan-cimd/1")
	response, err := client.Do(req)
	if err != nil {
		return Client{}, ErrInvalidClient
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != 200 || response.ContentLength > 65536 {
		return Client{}, ErrInvalidClient
	}
	for _, encoding := range response.Header.Values("Content-Encoding") {
		if !strings.EqualFold(strings.TrimSpace(encoding), "identity") {
			return Client{}, ErrInvalidClient
		}
	}
	kind, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if len(response.Header.Values("Content-Type")) != 1 || err != nil || (kind != "application/json" && (!strings.HasPrefix(kind, "application/") || !strings.HasSuffix(kind, "+json") || len(kind) <= len("application/+json"))) {
		return Client{}, ErrInvalidClient
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, 65537))
	if err != nil || len(raw) > 65536 || ctx.Err() != nil {
		return Client{}, ErrInvalidClient
	}
	return parseCIMD(raw, id)
}
