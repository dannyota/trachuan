//go:build integration

// Package testtemporal shares a Temporal development server in a test binary.
package testtemporal

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/testsuite"
)

var once sync.Once
var server *testsuite.DevServer
var startErr error
var counter atomic.Uint64

// Server starts the package's shared server on first use.
func Server(t *testing.T) *testsuite.DevServer {
	t.Helper()
	once.Do(func() {
		server, startErr = testsuite.StartDevServer(context.Background(), testsuite.DevServerOptions{ClientOptions: &client.Options{Namespace: "corpus"}})
	})
	if startErr != nil {
		t.Fatalf("start Temporal server: %v", startErr)
	}
	return server
}

// Release returns a release name unique within the test binary.
func Release() string { return fmt.Sprintf("20260101_%d", counter.Add(1)) }

// Stop stops the shared server after all tests have completed.
func Stop() error {
	if server == nil {
		return nil
	}
	return server.Stop()
}
