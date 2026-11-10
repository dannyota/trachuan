//go:build integration

package testpg

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

const processReleaseName = "testpg-cross-process-release"
const processReleaseEnv = "TRACHUAN_TEST_RELEASE_CHILD"

func TestRelease_ProcessHelper(t *testing.T) {
	if os.Getenv(processReleaseEnv) != "1" {
		t.Skip("child process only")
	}
	rel := releaseForName(t, processReleaseName)
	fmt.Println("release=" + rel)
	if _, err := bufio.NewReader(os.Stdin).ReadString('\n'); err != nil {
		t.Fatalf("wait for parent: %v", err)
	}
}

func TestRelease_DistinctAcrossProcesses(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 1800*time.Millisecond)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestRelease_ProcessHelper$")
	cmd.Env = append(os.Environ(), processReleaseEnv+"=1")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("open child stdin: %v", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("open child stdout: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start child test process: %v", err)
	}
	waited := false
	defer func() {
		if !waited {
			cancel()
			_ = cmd.Wait()
		}
	}()
	lines := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			if strings.HasPrefix(scanner.Text(), "release=") {
				lines <- strings.TrimPrefix(scanner.Text(), "release=")
				return
			}
		}
		close(lines)
	}()
	var child string
	select {
	case child = <-lines:
		if child == "" {
			t.Fatal("child test process exited without reporting a release")
		}
	case <-ctx.Done():
		t.Fatal("child test process did not report a release before timeout")
	}
	if parent := releaseForName(t, processReleaseName); parent == child {
		t.Errorf("parent and child reserved the same release %q", child)
	}
	if _, err := fmt.Fprintln(stdin, "done"); err != nil {
		t.Fatalf("signal child test process: %v", err)
	}
	if err := cmd.Wait(); err != nil {
		waited = true
		t.Fatalf("child test process failed: %v", err)
	}
	waited = true
	admin, _ := administrator(t)
	hash := sha256.Sum256([]byte("trachuan-test-release:" + child))
	key := int64(binary.BigEndian.Uint64(hash[:8]))
	var locked bool
	if err := admin.QueryRow(ctx, `SELECT pg_try_advisory_lock($1)`, key).Scan(&locked); err != nil || !locked {
		t.Fatalf("child lock after exit: locked=%v error=%v", locked, err)
	}
}
