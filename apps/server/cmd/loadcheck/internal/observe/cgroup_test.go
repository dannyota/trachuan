package observe

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func writeService(t *testing.T, root, name string, usec, current, peak uint64) {
	t.Helper()
	dir := filepath.Join(root, "trachuan-"+name+".service")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"cpu.stat":       fmt.Sprintf("usage_usec %d\nuser_usec 1\nsystem_usec 2\n", usec),
		"memory.current": fmt.Sprintf("%d\n", current),
		"memory.peak":    fmt.Sprintf("%d\n", peak),
	}
	for file, content := range files {
		if err := os.WriteFile(filepath.Join(dir, file), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestCgroup_CPUPercentFromTwoSamples(t *testing.T) {
	root := t.TempDir()
	reader := newCgroupReader(root, []string{"web"})
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	writeService(t, root, "web", 1_000_000, 100, 150)
	first, err := reader.read(at)
	if err != nil {
		t.Fatal(err)
	}
	if first["web"].CPU != nil || first["web"].Restart || first["web"].Current != 100 || first["web"].Peak != 150 {
		t.Fatalf("first %+v", first["web"])
	}
	writeService(t, root, "web", 2_500_000, 120, 200)
	second, err := reader.read(at.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	got := second["web"]
	if got.CPU == nil || *got.CPU != 150 || got.Restart || got.Current != 120 || got.Peak != 200 {
		t.Fatalf("second %+v", got)
	}
}

func TestCgroup_CounterResetIsRestart(t *testing.T) {
	root := t.TempDir()
	reader := newCgroupReader(root, []string{"web"})
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	writeService(t, root, "web", 5_000_000, 1, 1)
	if _, err := reader.read(at); err != nil {
		t.Fatal(err)
	}
	writeService(t, root, "web", 1_000, 1, 1)
	got, err := reader.read(at.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if !got["web"].Restart || got["web"].CPU != nil {
		t.Fatalf("reset %+v", got["web"])
	}
}

func TestCgroup_VanishedDirectoryThenNewOneIsRestart(t *testing.T) {
	root := t.TempDir()
	reader := newCgroupReader(root, []string{"web", "caddy"})
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	writeService(t, root, "web", 5_000_000, 1, 1)
	writeService(t, root, "caddy", 1, 1, 1)
	if _, err := reader.read(at); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(root, "trachuan-web.service")); err != nil {
		t.Fatal(err)
	}
	gone, err := reader.read(at.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := gone["web"]; ok || gone["caddy"].Restart {
		t.Fatalf("gone %+v", gone)
	}
	writeService(t, root, "web", 9_000_000, 1, 1)
	back, err := reader.read(at.Add(2 * time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if !back["web"].Restart || back["web"].CPU != nil {
		t.Fatalf("back %+v", back["web"])
	}
}

func TestCgroup_MalformedFileIsError(t *testing.T) {
	root := t.TempDir()
	writeService(t, root, "web", 1, 1, 1)
	if err := os.WriteFile(filepath.Join(root, "trachuan-web.service", "memory.peak"), []byte("max\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := newCgroupReader(root, []string{"web"}).read(time.Now()); err == nil {
		t.Fatal("malformed memory.peak accepted")
	}
}

func TestSample_CgroupServices(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "app.slice")
	writeService(t, root, "web", 1_000_000, 4096, 8192)
	ps := filepath.Join(dir, "ps")
	if err := os.WriteFile(ps, []byte("#!/bin/sh\nprintf '%s\\n' '123 Thu Jan  1 12:00:00 2026 100 1.5 fixture'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	if err := exec.Command("env", "LC_ALL=C", "ps").Run(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "samples.jsonl")
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if err := Sample(ctx, []string{"--out", path, "--cgroup-root", root, "--service", "web,missing"}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var got sample
	if err := json.Unmarshal([]byte(strings.SplitN(string(data), "\n", 2)[0]), &got); err != nil {
		t.Fatal(err)
	}
	web := got.Services["web"]
	if web.Current != 4096 || web.Peak != 8192 || web.Restart {
		t.Fatalf("web %+v", web)
	}
	if _, ok := got.Services["missing"]; ok {
		t.Fatal("missing service recorded")
	}
	if !strings.Contains(string(data), `"memory_current_bytes":4096`) || !strings.Contains(string(data), `"memory_peak_bytes":8192`) {
		t.Fatalf("json %s", data)
	}
}

func TestSample_ServiceNeedsRoot(t *testing.T) {
	err := Sample(context.Background(), []string{"--out", filepath.Join(t.TempDir(), "s.jsonl"), "--service", "web"})
	if err == nil {
		t.Fatal("service without cgroup root accepted")
	}
}

func TestCgroup_RemovedServiceErrorsCountAsAbsent(t *testing.T) {
	for _, err := range []error{&fs.PathError{Op: "read", Path: "x", Err: syscall.ENODEV}, &fs.PathError{Op: "open", Path: "x", Err: fs.ErrNotExist}} {
		if !serviceGone(err) {
			t.Errorf("%v not treated as gone", err)
		}
	}
	if serviceGone(&fs.PathError{Op: "read", Path: "x", Err: syscall.EACCES}) || serviceGone(nil) {
		t.Fatal("other errors treated as gone")
	}
}
