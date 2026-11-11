package app

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/credential"
	"danny.vn/trachuan/apps/server/internal/limits"
	"danny.vn/trachuan/apps/server/internal/mcpapi"
	"danny.vn/trachuan/apps/server/internal/store"
	"danny.vn/trachuan/internal/retrieval"
)

type slowBodyAuth struct {
	drainStore
	calls atomic.Int64
}

func (s *slowBodyAuth) GetCredential(ctx context.Context, hash string) (store.GetCredentialRow, error) {
	s.calls.Add(1)
	return s.drainStore.GetCredential(ctx, hash)
}

type slowBodyResolver struct{ calls atomic.Int64 }

func (s *slowBodyResolver) Resolve(context.Context, string, string) (mcpapi.Lease, error) {
	s.calls.Add(1)
	return mcpapi.Lease{}, retrieval.ErrUnavailable
}

func TestHTTPServer_UsesHostileClientBudgets(t *testing.T) {
	server := newHTTPServer("127.0.0.1:0", http.NotFoundHandler())
	if server.ReadHeaderTimeout != 5*time.Second || server.MaxHeaderBytes != 16<<10 || server.ReadTimeout != 10*time.Second || server.WriteTimeout != 30*time.Second || server.IdleTimeout != 120*time.Second {
		t.Fatal("HTTP budgets are incomplete")
	}
}

func TestHTTPServer_PartialAuthenticatedBodyClosesBeforeTool(t *testing.T) {
	auth := &slowBodyAuth{}
	resolver := &slowBodyResolver{}
	handler := credential.Middleware(&credential.Verifier{Store: auth, Audience: "http://localhost/mcp"}, mcpapi.New(mcpapi.Config{Resolver: resolver, Limiter: limits.New(time.Now)}))
	server := newHTTPServer("127.0.0.1:0", handler)
	// Shrink the production budgets while preserving a missing timeout as zero.
	server.ReadHeaderTimeout /= 500
	server.ReadTimeout /= 500
	server.WriteTimeout /= 500
	server.IdleTimeout /= 500
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	served := make(chan error, 1)
	go func() { served <- server.Serve(listener) }()
	t.Cleanup(func() {
		_ = server.Close()
		if serveErr := <-served; !errors.Is(serveErr, http.ErrServerClosed) {
			t.Error(serveErr)
		}
	})
	conn, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	if err = conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatal(err)
	}
	request := "POST /mcp HTTP/1.1\r\nHost: localhost\r\nAuthorization: Bearer trc_" + strings.Repeat("A", 43) + "\r\nContent-Type: application/json\r\nAccept: application/json, text/event-stream\r\nContent-Length: 1000\r\n\r\n{\"jsonrpc\":"
	if _, err = io.WriteString(conn, request); err != nil {
		t.Fatal(err)
	}
	if _, err = io.ReadAll(conn); err != nil && !errors.Is(err, syscall.ECONNRESET) {
		t.Fatal("server did not close the stalled body connection", err)
	}
	if auth.calls.Load() != 1 || resolver.calls.Load() != 0 {
		t.Fatal("authentication was skipped or the partial body ran a tool")
	}
}

func TestReadyCheck_AuthPingHasTwoSecondDeadline(t *testing.T) {
	bounded := false
	err := readyCheck(context.Background(), func(ctx context.Context) error {
		deadline, ok := ctx.Deadline()
		bounded = ok && time.Until(deadline) > 0 && time.Until(deadline) <= 2*time.Second
		return nil
	}, func(context.Context) error { return nil })
	if err != nil || !bounded {
		t.Fatal("readiness auth ping lacks the auth deadline", err)
	}
}
