package ocrvision

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func serviceCredentials(t *testing.T, endpoint string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal("generate fixture key")
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal("encode fixture key")
	}
	data, err := json.Marshal(map[string]string{"type": "service_account", "client_email": "test@example.invalid", "private_key": string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})), "token_uri": endpoint})
	if err != nil {
		t.Fatal("encode fixture credentials")
	}
	path := filepath.Join(t.TempDir(), "credentials.json")
	if err = os.WriteFile(path, data, 0600); err != nil {
		t.Fatal("write fixture credentials")
	}
	t.Setenv("GOOGLE_CLOUD_PROJECT", "test-project")
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", path)
}

func TestAuth_TokenEndpointBypassesProxy(t *testing.T) {
	var proxied, tokens atomic.Int64
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { proxied.Add(1); w.WriteHeader(502) }))
	defer proxy.Close()
	t.Setenv("HTTP_PROXY", proxy.URL)
	t.Setenv("HTTPS_PROXY", proxy.URL)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tokens.Add(1)
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"access_token":"test-token","token_type":"Bearer","expires_in":3600}`)
	}))
	defer server.Close()
	serviceCredentials(t, server.URL)
	transport := server.Client().Transport.(*http.Transport).Clone()
	proxyURL, err := url.Parse(proxy.URL)
	if err != nil {
		t.Fatal(err)
	}
	transport.Proxy = http.ProxyURL(proxyURL)
	_, err = newAuthenticated(context.Background(), &http.Client{Transport: transport}, 30*time.Second)
	if err != nil || proxied.Load() != 0 || tokens.Load() != 1 {
		t.Fatalf("err=%v proxy=%d tokens=%d", err, proxied.Load(), tokens.Load())
	}
	transport.CloseIdleConnections()
}

func TestAuth_StalledJWTEndpointHasOverallTimeout(t *testing.T) {
	started, ended, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		close(started)
		select {
		case <-release:
		case <-r.Context().Done():
		}
		close(ended)
	}))
	defer server.Close()
	defer close(release)
	serviceCredentials(t, server.URL)
	done := make(chan error, 1)
	go func() {
		_, err := newAuthenticated(context.Background(), server.Client(), 10*time.Millisecond)
		done <- err
	}()
	<-started
	select {
	case err := <-done:
		if err == nil {
			t.Error("stalled endpoint succeeded")
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("token acquisition ignored timeout")
	}
	select {
	case <-ended:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("JWT HTTP request outlived overall timeout")
	}
}

func TestAuth_StalledJWTEndpointHonorsCancellation(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		close(started)
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	defer server.Close()
	defer close(release)
	serviceCredentials(t, server.URL)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := newAuthenticated(ctx, server.Client(), 30*time.Second); done <- err }()
	<-started
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Error("canceled token accepted")
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("JWT token acquisition ignored cancellation")
	}
}
