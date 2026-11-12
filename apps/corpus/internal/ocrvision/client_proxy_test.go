package ocrvision

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

func TestClient_ProxyIsDisabled(t *testing.T) {
	var proxied, direct atomic.Int64
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { proxied.Add(1); w.WriteHeader(502) }))
	defer proxy.Close()
	t.Setenv("HTTP_PROXY", proxy.URL)
	t.Setenv("HTTPS_PROXY", proxy.URL)
	proxyURL, err := url.Parse(proxy.URL)
	if err != nil {
		t.Fatal(err)
	}
	for _, https := range []bool{false, true} {
		server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			direct.Add(1)
			_, _ = io.WriteString(w, `{"responses":[{}]}`)
		}))
		if https {
			server.StartTLS()
		} else {
			server.Start()
		}
		transport := server.Client().Transport.(*http.Transport).Clone()
		transport.Proxy = http.ProxyURL(proxyURL)
		client := NewClient(Config{BaseURL: server.URL, HTTP: &http.Client{Transport: transport}, Project: "test", TokenSource: oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "test"}), Wait: func(context.Context) error { return nil }, Sleep: func(context.Context, time.Duration) error { return nil }})
		_, requestErr := client.recognize(context.Background(), nil, newBudget(1))
		server.Close()
		transport.CloseIdleConnections()
		if requestErr != nil {
			t.Errorf("https=%v: direct call failed: %v", https, requestErr)
		}
	}
	if proxied.Load() != 0 || direct.Load() != 2 {
		t.Fatalf("proxy=%d direct=%d", proxied.Load(), direct.Load())
	}
	client := NewClient(Config{})
	transport, ok := client.http.Transport.(*http.Transport)
	if !ok || transport.Proxy != nil {
		t.Fatal("production client has no explicit proxy-free transport")
	}
}
