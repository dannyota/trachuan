package ocrcheck

import (
	"bytes"
	"context"
	"image/png"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

func nativeSandbox(t *testing.T) *PopplerRenderer {
	t.Helper()
	if _, err := discoverTools(runtime.GOOS, exec.LookPath, os.Stat); err != nil {
		t.Skip("native sandbox or trusted Poppler tools unavailable: " + err.Error())
	}
	dir := nativeScratch(t)
	// Distinguish a launcher that forbids sandbox initialization from a bad profile.
	if _, err := runCommand(context.Background(), dir, sandboxExecutable, []string{"-p", "(version 1)(allow default)", "/usr/bin/true"}, io.Discard); err != nil {
		t.Skip("execution environment refuses sandbox-exec initialization")
	}
	r := &PopplerRenderer{}
	if err := r.Preflight(context.Background()); err != nil {
		t.Fatal("native renderer sandbox preflight failed: " + err.Error())
	}
	return r
}

func nativeScratch(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return dir
}
func TestRenderer_NativeSandboxDeniesOutsideWrites(t *testing.T) {
	r := nativeSandbox(t)
	dir := nativeScratch(t)
	outside := filepath.Join(nativeScratch(t), "outside")
	// touch needs no shell and no files beyond the system libraries the profile allows.
	const touch = "/usr/bin/touch"
	profile := sandboxProfile(touch, dir)
	if _, err := runCommand(context.Background(), dir, r.tools.sandbox, []string{"-p", profile, touch, filepath.Join(dir, "inside")}, io.Discard); err != nil {
		t.Fatal("sandbox write control failed")
	}
	if _, err := os.Stat(filepath.Join(dir, "inside")); err != nil {
		t.Fatal("scratch write failed")
	}
	_, err := runCommand(context.Background(), dir, r.tools.sandbox, []string{"-p", profile, touch, outside}, io.Discard)
	if err == nil {
		t.Fatal("outside write allowed")
	}
	if _, err = os.Stat(outside); !os.IsNotExist(err) {
		t.Fatal("outside file created")
	}
}
func TestRenderer_NativeSandboxDeniesNetwork(t *testing.T) {
	r := nativeSandbox(t)
	dir := nativeScratch(t)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = listener.Close() }()
	port := strconv.Itoa(listener.Addr().(*net.TCPAddr).Port)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	args := []string{"-z", "-w", "1", "127.0.0.1", port}
	profile := sandboxProfile("/usr/bin/nc", dir)
	control := strings.Replace(profile, "(deny network*)", "(allow network*)", 1)
	if _, err = runCommand(ctx, dir, r.tools.sandbox, append([]string{"-p", control, "/usr/bin/nc"}, args...), io.Discard); err != nil {
		t.Fatal("local connection control failed")
	}
	if _, err = runCommand(ctx, dir, r.tools.sandbox, append([]string{"-p", profile, "/usr/bin/nc"}, args...), io.Discard); err == nil {
		t.Fatal("network allowed inside sandbox")
	}
}
func TestRenderer_NativeLargePageIsScaled(t *testing.T) {
	r := nativeSandbox(t)
	result, err := r.Render(context.Background(), pdfWithPageBox("72000 72"), maxRequestBytes)
	if err != nil || len(result.Pages) != 1 {
		t.Fatalf("large page: error=%v pages=%d", err, len(result.Pages))
	}
	config, err := png.DecodeConfig(bytes.NewReader(result.Pages[0]))
	if err != nil || config.Width != 3000 || config.Height > 3000 {
		t.Fatalf("large page raster=%+v error=%v", config, err)
	}
}
