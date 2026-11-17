package publicsummary

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type stalledPostgres struct {
	listener     net.Listener
	accepted     chan struct{}
	disconnected chan struct{}
	mu           sync.Mutex
	connections  []net.Conn
	done         chan struct{}
	workers      sync.WaitGroup
	once         sync.Once
}

func newStalledPostgres(t *testing.T) *stalledPostgres {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := &stalledPostgres{listener: listener, accepted: make(chan struct{}, 16), disconnected: make(chan struct{}, 16), done: make(chan struct{})}
	go func() {
		defer close(s.done)
		for {
			conn, acceptErr := listener.Accept()
			if acceptErr != nil {
				return
			}
			s.mu.Lock()
			s.connections = append(s.connections, conn)
			s.mu.Unlock()
			s.workers.Add(1)
			go func() {
				defer s.workers.Done()
				_, _ = io.Copy(io.Discard, conn)
				_ = conn.Close()
				s.disconnected <- struct{}{}
			}()
			s.accepted <- struct{}{}
		}
	}()
	t.Cleanup(s.Close)
	return s
}

func (s *stalledPostgres) URL() string {
	return fmt.Sprintf("postgres://synthetic:synthetic@%s/{database}?sslmode=disable", s.listener.Addr())
}
func (s *stalledPostgres) Close() {
	s.once.Do(func() {
		_ = s.listener.Close()
		<-s.done
		s.mu.Lock()
		for _, conn := range s.connections {
			_ = conn.Close()
		}
		s.mu.Unlock()
		s.workers.Wait()
	})
}

func TestPostgres_StalledHandshakeRespectsCallerCancellation(t *testing.T) {
	server := newStalledPostgres(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() {
		pool, err := OpenPostgres(server.URL())(ctx, "corpus_vn_20990101")
		if pool != nil {
			pool.Close()
		}
		result <- err
	}()
	if _, ok := receivePromptly(t, server.accepted, "connection attempt did not start"); !ok {
		server.Close()
		return
	}
	cancel()
	if err, ok := receivePromptly(t, result, "failed opening waited on a detached constructor"); ok && err == nil {
		t.Error("stalled handshake succeeded")
	}
	receivePromptly(t, server.disconnected, "cancelled constructor kept its socket open")
	server.Close()
}

func TestPostgres_RetirementCancelsStalledHandshake(t *testing.T) {
	server := newStalledPostgres(t)
	var observations atomic.Int64
	m := NewManager(func(context.Context) (Release, error) {
		name, projection := "20990101", 1
		if observations.Add(1) > 1 {
			name, projection = "20990102", 0
		}
		return Release{Database: "corpus_vn_" + name, ID: name, Projection: projection}, nil
	}, OpenPostgres(server.URL()), time.Now)
	defer m.Close()
	first := make(chan error, 1)
	go func() {
		lease, err := m.Resolve(context.Background())
		if lease != nil {
			lease.Done()
		}
		first <- err
	}()
	if _, ok := receivePromptly(t, server.accepted, "opening did not reach PostgreSQL"); !ok {
		server.Close()
		return
	}
	next, err := m.Resolve(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	next.Done()
	if err, ok := receivePromptly(t, first, "retired opening kept its request"); ok && err == nil {
		t.Error("retired opening succeeded")
	}
	receivePromptly(t, server.disconnected, "retirement did not cancel stalled construction")
	server.Close()
}

func TestSummary_HandshakeDisconnectsReleaseAdmission(t *testing.T) {
	server := newStalledPostgres(t)
	h, _, _, _ := handlerFixture(t)
	h.manager.open = OpenPostgres(server.URL())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		request := httptest.NewRequest("GET", "/api/v1/public/documents", nil).WithContext(ctx)
		h.ServeHTTP(httptest.NewRecorder(), request)
	}()
	if _, ok := receivePromptly(t, server.accepted, "opening did not start"); !ok {
		server.Close()
		return
	}
	cancel()
	receivePromptly(t, done, "disconnected handshake caller held admission")
	if len(h.gate) != 0 {
		t.Error("disconnected caller kept its admission slot")
	}
	h.manager.Close()
	receivePromptly(t, server.disconnected, "manager close left stalled socket")
	server.Close()
}
