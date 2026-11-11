package oauth

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func cimdFixture(id string) string {
	return `{"client_id":"` + id + `","client_name":"Fixture","redirect_uris":["http://localhost/cb"]}`
}

func tlsResolver(t *testing.T, handler http.HandlerFunc) *CIMDResolver {
	t.Helper()
	s := httptest.NewTLSServer(handler)
	t.Cleanup(s.Close)
	f := NewCIMDResolver()
	f.roots = x509.NewCertPool()
	f.roots.AddCert(s.Certificate())
	f.lookup = func(context.Context, string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("8.8.8.8")}, nil
	}
	f.dial = func(ctx context.Context, network, address string) (net.Conn, error) {
		if address != "8.8.8.8:443" {
			t.Errorf("dial target = %s", address)
		}
		return (&net.Dialer{}).DialContext(ctx, network, s.Listener.Addr().String())
	}
	return f
}

func TestCIMD_FetchPolicy(t *testing.T) {
	for _, address := range []string{"0.0.0.0", "10.0.0.1", "127.0.0.1", "169.254.1.1", "224.0.0.1", "::", "::1", "fe80::1", "ff02::1", "::ffff:8.8.8.8", "64:ff9b::808:808", "64:ff9b:1::1", "2002:808:808::1", "2001::1", "192.0.2.1", "198.51.100.1", "203.0.113.1", "198.18.0.1", "2001:db8::1", "100.64.0.1", "fc00::1", "3fff::1"} {
		t.Run(address, func(t *testing.T) {
			f := NewCIMDResolver()
			f.lookup = func(context.Context, string) ([]netip.Addr, error) {
				return []netip.Addr{netip.MustParseAddr("8.8.8.8"), netip.MustParseAddr(address)}, nil
			}
			f.dial = func(context.Context, string, string) (net.Conn, error) {
				t.Error("unsafe answer reached dial")
				return nil, fmt.Errorf("unexpected dial")
			}
			if _, err := f.Resolve(t.Context(), "https://example.com/client"); err == nil {
				t.Fatal("unsafe answer accepted")
			}
		})
	}
	for _, id := range []string{"https://127.0.0.1/client", "https://[::1]/client", "http://example.com/client", "https://user@example.com/client", "https://example.com/client#x"} {
		f := NewCIMDResolver()
		f.lookup = func(context.Context, string) ([]netip.Addr, error) {
			t.Error("invalid URL reached DNS")
			return nil, nil
		}
		if _, err := f.Resolve(t.Context(), id); err == nil {
			t.Errorf("accepted %s", id)
		}
	}
	for _, tc := range []struct {
		name   string
		status int
		body   string
		valid  bool
	}{
		{"valid", 200, cimdFixture("https://example.com/client"), true},
		{"redirect", 302, "", false},
		{"oversize", 200, strings.Repeat(" ", 65537), false},
		{"mismatch", 200, cimdFixture("https://example.com/other"), false},
		{"duplicate", 200, strings.TrimSuffix(cimdFixture("https://example.com/client"), "}") + `,"client_id":"https://evil.example/client"}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := tlsResolver(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Host != "example.com" || r.TLS.ServerName != "example.com" {
					t.Error("TLS identity or Host changed")
				}
				if r.Header.Get("Accept-Encoding") != "" || r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
					t.Error("unexpected credentials or compression")
				}
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Location", "https://example.com/other")
				w.WriteHeader(tc.status)
				_, _ = fmt.Fprint(w, tc.body)
			})
			_, err := f.Resolve(t.Context(), "https://example.com/client")
			if (err == nil) != tc.valid {
				t.Fatalf("error = %v", err)
			}
		})
	}
	t.Run("timeout includes DNS", func(t *testing.T) {
		f := NewCIMDResolver()
		f.timeout = 10 * time.Millisecond
		f.lookup = func(ctx context.Context, _ string) ([]netip.Addr, error) { <-ctx.Done(); return nil, ctx.Err() }
		if _, err := f.Resolve(t.Context(), "https://example.com/client"); err == nil {
			t.Fatal("timeout accepted")
		}
	})
	t.Run("slow body", func(t *testing.T) {
		f := tlsResolver(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = http.NewResponseController(w).Flush()
			<-r.Context().Done()
		})
		f.timeout = 20 * time.Millisecond
		if _, err := f.Resolve(t.Context(), "https://example.com/client"); err == nil {
			t.Fatal("slow body accepted")
		}
	})
}

func TestCIMD_MethodSelection(t *testing.T) {
	for _, tc := range []struct {
		field string
		valid bool
	}{
		{``, true}, {`,"token_endpoint_auth_method":"none"`, true}, {`,"token_endpoint_auth_methods_supported":["none"]`, true},
		{`,"token_endpoint_auth_method":"none","token_endpoint_auth_methods_supported":["none"]`, true},
		{`,"token_endpoint_auth_method":"client_secret_basic"`, false},
		{`,"token_endpoint_auth_methods_supported":["none","private_key_jwt"]`, false},
		{`,"token_endpoint_auth_methods_supported":[]`, false}, {`,"token_endpoint_auth_method":null`, false},
		{`,"token_endpoint_auth_methods_supported":null`, false},
	} {
		t.Run(tc.field, func(t *testing.T) {
			raw := strings.TrimSuffix(cimdFixture("https://example.com/client"), "}") + tc.field + "}"
			_, err := parseCIMD([]byte(raw), "https://example.com/client")
			if (err == nil) != tc.valid {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestCIMD_CacheBound(t *testing.T) {
	var calls atomic.Int32
	now := time.Date(2026, 11, 11, 0, 0, 0, 0, time.UTC)
	f := tlsResolver(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, cimdFixture("https://example.com"+r.URL.Path))
	})
	f.clock = func() time.Time { return now }
	for i := range 1001 {
		if _, err := f.Resolve(t.Context(), fmt.Sprintf("https://example.com/%d", i)); err != nil {
			t.Fatal(err)
		}
	}
	c, err := f.Resolve(t.Context(), "https://example.com/1000")
	if err != nil {
		t.Fatal(err)
	}
	c.RedirectURIs[0] = "https://evil.example/cb"
	c, err = f.Resolve(t.Context(), "https://example.com/1000")
	if err != nil || c.RedirectURIs[0] != "http://localhost/cb" {
		t.Fatal("cache caller mutated metadata")
	}
	if calls.Load() != 1001 {
		t.Fatal("cache hit fetched")
	}
	if _, err = f.Resolve(t.Context(), "https://example.com/0"); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1002 {
		t.Fatal("least recent entry not evicted")
	}
	now = now.Add(24 * time.Hour)
	if _, err = f.Resolve(t.Context(), "https://example.com/0"); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1003 {
		t.Fatal("expired metadata reused")
	}
}

func TestCIMD_ConcurrentBound(t *testing.T) {
	f := NewCIMDResolver()
	entered := make(chan struct{}, 4)
	release := make(chan struct{})
	f.lookup = func(ctx context.Context, _ string) ([]netip.Addr, error) {
		entered <- struct{}{}
		select {
		case <-release:
		case <-ctx.Done():
		}
		return nil, fmt.Errorf("fixture")
	}
	done := make(chan struct{}, 4)
	for i := range 4 {
		go func() { _, _ = f.Resolve(t.Context(), fmt.Sprintf("https://example.com/%d", i)); done <- struct{}{} }()
	}
	for range 4 {
		<-entered
	}
	_, err := f.Resolve(t.Context(), "https://example.com/fifth")
	close(release)
	for range 4 {
		<-done
	}
	if err == nil {
		t.Fatal("fifth concurrent fetch admitted")
	}
}

func TestCIMD_ValidatedDial(t *testing.T) {
	f := NewCIMDResolver()
	for _, target := range []string{"127.0.0.1:443", "8.8.8.8:80", "[::ffff:8.8.8.8]:443", "example.com:443", "1.1.1.1:443"} {
		if f.checkDial(target, 443, []netip.Addr{netip.MustParseAddr("8.8.8.8")}) == nil {
			t.Errorf("accepted %s", target)
		}
	}
	if err := f.checkDial("8.8.8.8:443", 443, []netip.Addr{netip.MustParseAddr("8.8.8.8")}); err != nil {
		t.Fatal(err)
	}
}

func TestCIMD_JSONAndRedirectBounds(t *testing.T) {
	for _, value := range []string{`null`, `[]`, `{"client_id":"https://example.com/client","redirect_uris":null}`, strings.TrimSuffix(cimdFixture("https://example.com/client"), "}") + `,"client_secret":"refused"}`} {
		if _, err := parseCIMD([]byte(value), "https://example.com/client"); err == nil {
			t.Fatal("invalid document accepted")
		}
	}
	var fields map[string]any
	_ = json.Unmarshal([]byte(cimdFixture("https://example.com/client")), &fields)
	fields["redirect_uris"] = []string{"https://example.com/cb#fragment"}
	raw, _ := json.Marshal(fields)
	if _, err := parseCIMD(raw, "https://example.com/client"); err == nil {
		t.Fatal("bad redirect accepted")
	}
}

func TestCIMD_DeadlineCoversDial(t *testing.T) {
	f := NewCIMDResolver()
	f.timeout = 20 * time.Millisecond
	f.lookup = func(context.Context, string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("8.8.8.8")}, nil
	}
	deadlineSeen := make(chan bool, 1)
	f.dial = func(ctx context.Context, _, _ string) (net.Conn, error) {
		_, ok := ctx.Deadline()
		deadlineSeen <- ok
		return nil, fmt.Errorf("fixture refuses connection")
	}
	_, _ = f.Resolve(t.Context(), "https://example.com/client")
	if !<-deadlineSeen {
		t.Fatal("fetch deadline stripped before dialing")
	}
}

func TestCIMD_DNSRebindingAndTLS(t *testing.T) {
	var lookups atomic.Int32
	f := tlsResolver(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, cimdFixture("https://example.com/client"))
	})
	f.lookup = func(context.Context, string) ([]netip.Addr, error) {
		if lookups.Add(1) > 1 {
			return []netip.Addr{netip.MustParseAddr("127.0.0.1")}, nil
		}
		return []netip.Addr{netip.MustParseAddr("8.8.8.8")}, nil
	}
	if _, err := f.Resolve(t.Context(), "https://example.com/client"); err != nil || lookups.Load() != 1 {
		t.Fatalf("pinned fetch err %v lookups %d", err, lookups.Load())
	}
	// No trusted certificate means HTTP must never be reached.
	bad := tlsResolver(t, func(http.ResponseWriter, *http.Request) { t.Error("untrusted TLS reached HTTP") })
	bad.roots = x509.NewCertPool()
	if _, err := bad.Resolve(t.Context(), "https://example.com/client"); err == nil {
		t.Fatal("untrusted TLS accepted")
	}
}

func TestCIMD_ResponseBounds(t *testing.T) {
	for _, tc := range []struct {
		name, contentType, encoding string
		size                        int
		chunked, valid              bool
	}{
		{"exact limit", "application/json", "", 65536, false, true},
		{"chunked oversize", "application/json", "", 65537, true, false},
		{"compression", "application/json", "gzip", 0, false, false},
		{"wrong media type", "text/html", "", 0, false, false},
		{"JSON subtype", "application/client+json", "", 0, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var requests atomic.Int32
			f := tlsResolver(t, func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				w.Header().Set("Content-Type", tc.contentType)
				if tc.encoding != "" {
					w.Header().Set("Content-Encoding", tc.encoding)
				}
				if tc.chunked {
					_ = http.NewResponseController(w).Flush()
				}
				body := cimdFixture("https://example.com/client")
				if tc.size > len(body) {
					body += strings.Repeat(" ", tc.size-len(body))
				}
				_, _ = fmt.Fprint(w, body)
			})
			for range 2 {
				_, err := f.Resolve(t.Context(), "https://example.com/client")
				if (err == nil) != tc.valid {
					t.Fatalf("fetch error %v", err)
				}
			}
			want := int32(2)
			if tc.valid {
				want = 1
			}
			if requests.Load() != want {
				t.Fatal("failed document cached or successful document refetched")
			}
		})
	}
}
