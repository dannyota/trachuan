//go:build unix

package app

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
)

func TestAdminSession_RestrictiveUmask(t *testing.T) {
	if os.Getenv("TRACHUAN_SESSION_UMASK_CHILD") != "1" {
		command := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestAdminSession_RestrictiveUmask$", "-test.count=1")
		command.Env = append(os.Environ(), "TRACHUAN_SESSION_UMASK_CHILD=1")
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("umask subprocess failed: %v\n%s", err, output)
		}
		return
	}
	path := filepath.Join(t.TempDir(), "session")
	s := newAdminSessionCommitFixture(path)
	previous := syscall.Umask(0777)
	defer syscall.Umask(previous)
	var out, stderr bytes.Buffer
	if code := exitCode(runAdmin(context.Background(), []string{"session", "create", "--account", "42", "--out", path}, s, &out), &stderr); code != 0 || out.Len() != 0 || stderr.Len() != 0 {
		t.Fatal("session creation under restrictive umask failed")
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("restrictive umask changed session mode from 0600")
	}
	if data, readErr := os.ReadFile(path); readErr != nil || len(data) != 44 {
		t.Fatal("session file is not readable by its owner")
	}
}
