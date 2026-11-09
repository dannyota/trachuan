//go:build integration && faultinject

package proof

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/migrate"
	"danny.vn/trachuan/apps/corpus/internal/testdb"
	"danny.vn/trachuan/apps/corpus/internal/testtemporal"
	"github.com/jackc/pgx/v5/pgxpool"
	workflowservice "go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/sdk/testsuite"
	"google.golang.org/protobuf/types/known/durationpb"
)

const release = "20261109"

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "corpus-proof-binary-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	binary := filepath.Join(dir, "corpus")
	cmd := exec.Command("go", "build", "-tags", "faultinject", "-o", binary, "./cmd/corpus")
	cmd.Dir = "../.."
	cmd.Env = append(os.Environ(), "GOPROXY=off", "GOSUMDB=off", "GOTOOLCHAIN=path")
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err = cmd.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		_ = os.RemoveAll(dir)
		os.Exit(1)
	}
	if err = os.Setenv("CORPUS_PROOF_BINARY", binary); err != nil {
		fmt.Fprintln(os.Stderr, err)
		_ = os.RemoveAll(dir)
		os.Exit(1)
	}
	code := m.Run()
	if err := testtemporal.Stop(); err != nil {
		fmt.Fprintln(os.Stderr, "stop Temporal server:", err)
		code = 1
	}
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

type harness struct {
	t                                                    *testing.T
	ctx                                                  context.Context
	dbURL, root, artifacts, checkpoints, dump, namespace string
	db                                                   *pgxpool.Pool
	server                                               *testsuite.DevServer
	gateway                                              *gateway
	worker                                               *exec.Cmd
	workerDone                                           chan error
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	h := &harness{t: t, ctx: context.Background(), root: t.TempDir()}
	h.artifacts = filepath.Join(h.root, "artifacts")
	h.checkpoints = filepath.Join(h.root, "checkpoints")
	h.dump = os.Getenv("CORPUS_PG_DUMP")
	if h.dump == "" {
		h.dump = "pg_dump"
	}
	adminURL := os.Getenv("CORPUS_TEST_ADMIN_URL")
	if adminURL != "" {
		parsed, parseErr := url.Parse(adminURL)
		if parseErr != nil {
			t.Fatal("invalid test database URL")
		}
		host := parsed.Hostname()
		ip := net.ParseIP(host)
		if host != "localhost" && (ip == nil || !ip.IsLoopback()) {
			t.Fatal("test database must use loopback")
		}
	}
	h.dbURL = testdb.New(t)
	if err := migrate.Up(h.ctx, h.dbURL); err != nil {
		t.Fatal(err)
	}
	var err error
	h.db, err = pgxpool.New(h.ctx, h.dbURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { h.db.Close() })
	h.server = testtemporal.Server(t)
	h.namespace = "proof-" + testtemporal.Release()
	_, err = h.server.Client().WorkflowService().RegisterNamespace(h.ctx, &workflowservice.RegisterNamespaceRequest{Namespace: h.namespace, WorkflowExecutionRetentionPeriod: durationpb.New(24 * time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	h.gateway = newGateway(t)
	if err := os.Mkdir(filepath.Join(h.root, "sources"), 0700); err != nil {
		t.Fatal(err)
	}
	sources := "country: vn\nvbpl:\n  sweep_agencies: ['62']\n  keyword_agencies: ['62']\n  keywords: ['test']\nrelation_codes:\n  '10':\n    kind: amends_supplements\n    label: amends\nstatus_codes: {}\n"
	if err := os.WriteFile(filepath.Join(h.root, "sources", "vn.yaml"), []byte(sources), 0600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if h.worker != nil {
			h.stopWorker()
		}
	})
	return h
}

func (h *harness) env(fault string) []string {
	return append(os.Environ(), "CORPUS_DATABASE_URL="+h.dbURL, "CORPUS_TEMPORAL_ADDRESS="+h.server.FrontendHostPort(), "CORPUS_TEMPORAL_NAMESPACE="+h.namespace, "CORPUS_ARTIFACTS="+h.artifacts, "CORPUS_CHECKPOINT_DIR="+h.checkpoints, "CORPUS_PG_DUMP="+h.dump, "CORPUS_VBPL_URL="+h.gateway.server.URL, "CORPUS_VBPL_LOOPBACK_HTTP=true", "CORPUS_VBPL_FILE_HOSTS=127.0.0.1", "CORPUS_HEARTBEAT_EVERY=200ms", "CORPUS_HEARTBEAT_TIMEOUT=2s", "CORPUS_VBPL_RPS=1000", "CORPUS_VBPL_IN_FLIGHT=16", "CORPUS_FAULT="+fault)
}

func (h *harness) command(args ...string) {
	h.t.Helper()
	cmd := exec.Command(os.Getenv("CORPUS_PROOF_BINARY"), args...)
	cmd.Dir, cmd.Env = h.root, h.env("")
	out, err := cmd.CombinedOutput()
	if err != nil {
		h.t.Fatalf("corpus %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

func (h *harness) startWorker(fault string) {
	h.t.Helper()
	cmd := exec.Command(os.Getenv("CORPUS_PROOF_BINARY"), "worker")
	cmd.Dir, cmd.Env = h.root, h.env(fault)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Start(); err != nil {
		h.t.Fatal(err)
	}
	h.worker = cmd
	h.workerDone = make(chan error, 1)
	go func() { h.workerDone <- cmd.Wait() }()
}

func (h *harness) stopWorker() {
	h.t.Helper()
	if err := h.worker.Process.Kill(); err != nil {
		h.t.Fatal(err)
	}
	<-h.workerDone
	h.worker = nil
}

func (h *harness) waitState(state string) {
	h.t.Helper()
	deadline := time.Now().Add(3 * time.Minute)
	for time.Now().Before(deadline) {
		var got string
		err := h.db.QueryRow(h.ctx, "SELECT state FROM run WHERE country='vn' AND rel=$1", release).Scan(&got)
		if err == nil && got == state {
			h.command("status", "vn", release)
			return
		}
		if err == nil && got == "failed" {
			h.t.Fatal("run entered failed state")
		}
		time.Sleep(10 * time.Millisecond)
	}
	h.t.Fatalf("timed out waiting for state %s", state)
}

func (h *harness) waitFetchProgress() {
	h.t.Helper()
	deadline := time.Now().Add(3 * time.Minute)
	for time.Now().Before(deadline) {
		var count int
		if err := h.db.QueryRow(h.ctx, "SELECT count(*) FROM run_item WHERE stage='fetch' AND state='done'").Scan(&count); err != nil {
			h.t.Fatal(err)
		}
		if count > 0 && count < 12 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	h.t.Fatal("timed out waiting for fetch progress")
}

func uninterrupted(t *testing.T) counts {
	t.Helper()
	h := newHarness(t)
	h.startWorker("")
	h.command("start", "vn", release, "--until", "relate")
	h.waitState("paused")
	return h.countRows()
}

func proveCrash(t *testing.T, point string) {
	t.Helper()
	baseline := uninterrupted(t)
	h := newHarness(t)
	h.startWorker(point)
	h.command("start", "vn", release, "--until", "relate")
	select {
	case err := <-h.workerDone:
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) || exitErr.ExitCode() != 3 {
			t.Fatalf("fault worker exited: %v", err)
		}
		h.worker = nil
	case <-time.After(3 * time.Minute):
		t.Fatal("fault worker did not exit")
	}
	var committed int
	if err := h.db.QueryRow(h.ctx, "SELECT count(*) FROM run_item WHERE stage='fetch' AND state='done'").Scan(&committed); err != nil {
		t.Fatal(err)
	}
	want := 7
	if point == "after_commit:7" {
		want = 8
	}
	if committed != want {
		t.Fatalf("committed fetch items at crash = %d, want %d", committed, want)
	}
	h.startWorker("")
	// A lost activity retries after its heartbeat timeout and backoff.
	h.waitFetchProgressAfterRetry()
	h.waitState("paused")
	h.assertCounts(baseline)
}

func (h *harness) waitFetchProgressAfterRetry() {
	h.t.Helper()
	deadline := time.Now().Add(3 * time.Minute)
	for time.Now().Before(deadline) {
		var count int
		if err := h.db.QueryRow(h.ctx, "SELECT count(*) FROM run_item WHERE stage='fetch' AND state='done'").Scan(&count); err != nil {
			h.t.Fatal(err)
		}
		if count > 8 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	h.t.Fatal("timed out waiting for retry")
}
