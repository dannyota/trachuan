//go:build integration

package publicsummary

import (
	"context"
	"testing"
	"time"
)

func TestPostgres_HandshakeTimeoutUsesStatementBudget(t *testing.T) {
	server := newStalledPostgres(t)
	result := make(chan error, 1)
	started := time.Now()
	go func() {
		pool, err := OpenPostgres(server.URL()+"&connect_timeout=60")(context.Background(), "corpus_vn_20990101")
		if pool != nil {
			pool.Close()
		}
		result <- err
	}()
	if _, ok := receivePromptly(t, server.accepted, "handshake did not start"); !ok {
		server.Close()
		return
	}
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("stalled handshake succeeded")
		}
	case <-time.After(2500 * time.Millisecond):
		server.Close()
		t.Fatal("connection attempt exceeded statement budget")
	}
	if elapsed := time.Since(started); elapsed > 2500*time.Millisecond {
		t.Fatalf("opening lasted %v", elapsed)
	}
	receivePromptly(t, server.disconnected, "timed out attempt retained a socket")
}
