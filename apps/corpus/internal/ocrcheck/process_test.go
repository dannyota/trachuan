package ocrcheck

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

type readyWriter struct {
	ready  chan int
	cancel context.CancelFunc
	dir    string
}

func (w readyWriter) Write(p []byte) (int, error) {
	if strings.Contains(string(p), "ready") {
		data, _ := os.ReadFile(filepath.Join(w.dir, "child.pid"))
		pid, _ := strconv.Atoi(string(data))
		w.ready <- pid
		w.cancel()
	}
	return len(p), nil
}

func TestRenderer_CancellationKillsDescendantsAndRemovesScratch(t *testing.T) {
	script := filepath.Join(t.TempDir(), "fake-renderer")
	if err := os.WriteFile(script, []byte("#!/bin/sh\n/bin/sleep 30 &\nprintf '%s' \"$!\" > \"$HOME/child.pid\"\nprintf ready\nwait\n"), 0700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ready := make(chan int, 1)
	scratch := make(chan string, 1)
	done := make(chan error, 1)
	renderer := PopplerRenderer{tools: &popplerTools{}, command: func(ctx context.Context, dir, _ string, _ []string, _ io.Writer) (RenderStats, error) {
		scratch <- dir
		tools := popplerTools{sandbox: script}
		return tools.command(ctx, dir, "ignored", nil, readyWriter{ready, cancel, dir})
	}}
	go func() { _, err := renderer.Render(ctx, smallPDF(), maxRequestBytes); done <- err }()
	dir := <-scratch
	var pid int
	select {
	case pid = <-ready:
	case <-time.After(time.Second):
		t.Fatal("fake renderer did not start")
	}
	if pid <= 0 {
		t.Fatal("missing descendant pid")
	}
	var err error
	defer func() { _ = unix.Kill(pid, unix.SIGKILL) }()
	select {
	case err = <-done:
		if err == nil {
			t.Fatal("canceled render accepted")
		}
	case <-time.After(75 * time.Millisecond):
		_ = unix.Kill(pid, unix.SIGKILL)
		err = <-done
		t.Fatalf("renderer blocked on a descendant holding the pipes: %v", err)
	}
	deadline := time.Now().Add(75 * time.Millisecond)
	for unix.Kill(pid, 0) == nil && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if unix.Kill(pid, 0) == nil {
		t.Fatal("renderer descendant survived cancellation")
	}
	if _, err = os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("scratch survived cancellation")
	}
}

type startedWriter struct{ ready chan struct{} }

func (w startedWriter) Write(p []byte) (int, error) { close(w.ready); return len(p), nil }

func TestRenderer_ScratchLimitStopsActiveProcess(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(t.TempDir(), "fake-renderer")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nprintf ready\n/bin/sleep 30\n"), 0700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ready := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		_, err := (&popplerTools{sandbox: script}).command(ctx, dir, "ignored", nil, startedWriter{ready})
		done <- err
	}()
	<-ready
	f, err := os.Create(filepath.Join(dir, "excess"))
	if err != nil {
		t.Fatal(err)
	}
	if err = f.Truncate(256_000_000); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	select {
	case err = <-done:
		if err == nil || err.Error() != "over_check_limit" {
			t.Fatalf("limit reason=%v", err)
		}
	case <-time.After(75 * time.Millisecond):
		cancel()
		err = <-done
		t.Fatalf("scratch growth did not stop active renderer: %v", err)
	}
}
