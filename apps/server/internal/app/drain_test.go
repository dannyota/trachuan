package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/credential"
	"danny.vn/trachuan/apps/server/internal/limits"
	"danny.vn/trachuan/apps/server/internal/mcpapi"
	"danny.vn/trachuan/apps/server/internal/store"
	"danny.vn/trachuan/internal/retrieval"
)

type drainEngine struct{ entered, proceed chan struct{} }

func (e *drainEngine) Status(context.Context) (retrieval.CorpusStatus, error) {
	close(e.entered)
	<-e.proceed
	return retrieval.CorpusStatus{}, nil
}
func (*drainEngine) Search(context.Context, retrieval.Request) (retrieval.Response, error) {
	return retrieval.Response{}, nil
}
func (*drainEngine) Present(context.Context, retrieval.Response, retrieval.Detail) (retrieval.SearchResult, error) {
	return retrieval.SearchResult{}, nil
}
func (*drainEngine) Read(context.Context, retrieval.ReadRequest) (retrieval.ReadResult, error) {
	return retrieval.ReadResult{}, nil
}
func (*drainEngine) Gaps(context.Context, string, int64, int) ([]retrieval.Gap, int64, error) {
	return nil, 0, nil
}

type drainResolver struct {
	engine                      *drainEngine
	mu                          sync.Mutex
	leased, closed, closedEarly bool
}

func (r *drainResolver) Resolve(context.Context, string, string) (mcpapi.Lease, error) {
	r.mu.Lock()
	r.leased = true
	r.mu.Unlock()
	return mcpapi.Lease{Engine: r.engine, Release: "20990101", Done: func() { r.mu.Lock(); r.leased = false; r.mu.Unlock() }}, nil
}
func (r *drainResolver) close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.closedEarly = r.leased
	r.closed = true
}

type drainStore struct{}

func (drainStore) GetCredential(context.Context, string) (store.GetCredentialRow, error) {
	return store.GetCredentialRow{AccountID: 1, ExpiresAt: time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC)}, nil
}
func (drainStore) TouchPersonalToken(context.Context, string) error { return nil }

type drainingServer struct{ stopped, requestDone, shutdownStarted chan struct{} }

func (s *drainingServer) ListenAndServe() error { <-s.stopped; return http.ErrServerClosed }
func (s *drainingServer) Shutdown(ctx context.Context) error {
	close(s.shutdownStarted)
	select {
	case <-s.requestDone:
		close(s.stopped)
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (*drainingServer) Close() error { return nil }

func TestRun_DrainOnSignal(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	e := &drainEngine{entered: make(chan struct{}), proceed: make(chan struct{})}
	resolver := &drainResolver{engine: e}
	server := &drainingServer{stopped: make(chan struct{}), requestDone: make(chan struct{}), shutdownStarted: make(chan struct{})}
	v := &credential.Verifier{Store: drainStore{}, Audience: "http://localhost/mcp", Clock: func() time.Time { return time.Date(2098, 1, 1, 0, 0, 0, 0, time.UTC) }}
	handler := credential.Middleware(v, mcpapi.New(mcpapi.Config{Resolver: resolver, Limiter: limits.New(time.Now)}))
	ready := readiness(ctx, func(context.Context) error { return nil })
	result := make(chan error, 1)
	go func() { result <- runHTTP(ctx, server, resolver.close) }()
	request := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"corpus_status","arguments":{"country":"vn"}}}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")
	request.Header.Set("Authorization", "Bearer trc_"+strings.Repeat("A", 43))
	response := httptest.NewRecorder()
	go func() { handler.ServeHTTP(response, request); close(server.requestDone) }()
	<-e.entered
	cancel()
	<-server.shutdownStarted
	w := httptest.NewRecorder()
	ready.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("readiness: %d", w.Code)
	}
	resolver.mu.Lock()
	closed := resolver.closed
	resolver.mu.Unlock()
	if closed {
		t.Error("resolver closed before request completed")
	}
	close(e.proceed)
	if err := <-result; err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"release":"20990101"`) {
		t.Fatal("in-flight request did not complete")
	}
	resolver.mu.Lock()
	defer resolver.mu.Unlock()
	if !resolver.closed || resolver.closedEarly {
		t.Fatal("resolver closed before lease finished")
	}
}
