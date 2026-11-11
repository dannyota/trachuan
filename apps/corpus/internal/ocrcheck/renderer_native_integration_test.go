//go:build darwin && integration

package ocrcheck

import (
	"bytes"
	"context"
	"image/png"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// These regressions must fail, rather than skip, if an installed sandbox fails.
func requiredNativeRenderer(t *testing.T) *PopplerRenderer {
	t.Helper()
	for _, tool := range []string{"pdfinfo", "pdftoppm"} {
		if _, err := exec.LookPath(tool); err != nil {
			if os.IsNotExist(err) || err == exec.ErrNotFound {
				t.Skip("required Poppler tool absent: " + tool)
			}
			t.Fatal("tool resolution failed")
		}
	}
	if _, err := os.Stat(sandboxExecutable); os.IsNotExist(err) {
		t.Skip("sandbox-exec absent")
	}
	r := &PopplerRenderer{}
	if err := r.Preflight(context.Background()); err != nil {
		t.Fatal(err)
	}
	return r
}
func TestRenderer_NativeCropBoxBound(t *testing.T) {
	r := requiredNativeRenderer(t)
	called := false
	r.command = func(ctx context.Context, dir, tool string, args []string, out io.Writer) (RenderStats, error) {
		if tool == r.tools.raster {
			called = true
			scale := slices.Index(args, "-scale-to")
			dpiIndex := slices.Index(args, "-r")
			dpi, parseErr := strconv.ParseFloat(args[dpiIndex+1], 64)
			if !slices.Contains(args, "-cropbox") || scale < 0 || scale+1 >= len(args) || args[scale+1] != "15" || parseErr != nil || dpi > 15 {
				t.Fatal("unsafe raster requested for mismatched boxes")
			}
		}
		return r.tools.command(ctx, dir, tool, args, out)
	}
	pdf := pdfWithBoxes("0 0 14400 14400", "0 0 72 72")
	result, err := r.Render(context.Background(), pdf, maxRequestBytes)
	if err != nil || !called || len(result.Pages) != 1 {
		t.Fatalf("bounded render: pages=%d error=%v", len(result.Pages), err)
	}
	config, err := png.DecodeConfig(bytes.NewReader(result.Pages[0]))
	if err != nil || config.Width > 3000 || config.Height > 3000 {
		t.Fatalf("raster exceeds bound: config=%+v error=%v", config, err)
	}
}
func TestRenderer_NativeDeniesOutsideReads(t *testing.T) {
	r := requiredNativeRenderer(t)
	const reader = "/bin/cat"
	if _, err := os.Stat(reader); os.IsNotExist(err) {
		t.Skip("test reader absent")
	}
	dir := nativeScratch(t)
	// Replace only occurrences of the selected executable literal. Every other
	// production read/write/network rule and dependency literal stays identical.
	production := sandboxProfile(r.tools.info, dir, r.tools.access)
	profile := strings.ReplaceAll(production, strconv.Quote(r.tools.info), strconv.Quote(reader))
	read := func(path string) error {
		_, err := runCommand(context.Background(), dir, r.tools.sandbox, []string{"-p", profile, reader, path}, io.Discard)
		return err
	}
	control := filepath.Join(dir, "control.txt")
	if err := os.WriteFile(control, []byte("synthetic sandbox read control"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := read(control); err != nil {
		t.Fatal("reader could not read the scratch control")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal("home directory unavailable")
	}
	outside, err := os.MkdirTemp(home, ".trachuan-sandbox-read-")
	if err != nil {
		t.Fatal("home probe creation failed")
	}
	defer func() { _ = os.RemoveAll(outside) }()
	homeFile := filepath.Join(outside, "probe.txt")
	if err = os.WriteFile(homeFile, []byte("synthetic outside read probe"), 0600); err != nil {
		t.Fatal("home probe write failed")
	}
	if err = read(homeFile); err == nil {
		t.Fatal("home file readable inside sandbox")
	}
	packageName := func(path string) string {
		const root = "/opt/homebrew/Cellar/"
		if !strings.HasPrefix(path, root) {
			return ""
		}
		parts := strings.Split(strings.TrimPrefix(path, root), "/")
		if len(parts) < 3 {
			return ""
		}
		return parts[0]
	}
	runtimePackages := map[string]bool{}
	for _, path := range append(slices.Clone(r.tools.access.files), r.tools.access.dirs...) {
		if name := packageName(path); name != "" {
			runtimePackages[name] = true
		}
	}
	candidates, _ := filepath.Glob("/opt/homebrew/Cellar/*/*/LICENSE*")
	for _, path := range candidates {
		real, resolveErr := filepath.EvalSymlinks(path)
		if resolveErr != nil || packageName(real) == "" || runtimePackages[packageName(real)] || runtimeReadAllowed(real, r.tools.access) {
			continue
		}
		file, openErr := os.Open(real)
		if openErr != nil {
			continue
		}
		info, statErr := file.Stat()
		_ = file.Close()
		if statErr != nil || !info.Mode().IsRegular() {
			continue
		}
		if err = read(real); err == nil {
			t.Fatal("unrelated Homebrew file readable inside sandbox")
		}
		return
	}
	t.Log("no readable unrelated Homebrew package license exists; package assertion skipped")
}
func runtimeReadAllowed(path string, access runtimeAccess) bool {
	if slices.Contains(access.files, path) {
		return true
	}
	for _, dir := range access.dirs {
		if path == dir || strings.HasPrefix(path, dir+string(filepath.Separator)) {
			return true
		}
	}
	return false
}
