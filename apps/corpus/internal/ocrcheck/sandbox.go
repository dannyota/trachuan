package ocrcheck

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const sandboxExecutable = "/usr/bin/sandbox-exec"

type popplerTools struct{ info, raster, sandbox string }

func rendererConfiguration() error {
	return fmt.Errorf("OCR renderer configuration requires trusted Poppler tools and an enforceable macOS sandbox")
}

func discoverTools(goos string, lookPath func(string) (string, error), stat func(string) (os.FileInfo, error)) (*popplerTools, error) {
	if goos != "darwin" {
		return nil, rendererConfiguration()
	}
	info, err := stat(sandboxExecutable)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0111 == 0 {
		return nil, rendererConfiguration()
	}
	tools := &popplerTools{sandbox: sandboxExecutable}
	for _, tool := range []struct {
		name string
		path *string
	}{{"pdfinfo", &tools.info}, {"pdftoppm", &tools.raster}} {
		path, lookupErr := lookPath(tool.name)
		if lookupErr != nil || !filepath.IsAbs(path) {
			return nil, rendererConfiguration()
		}
		// PATH entries may select only these three installation directories.
		switch filepath.Dir(path) {
		case "/opt/homebrew/bin", "/usr/local/bin", "/usr/bin":
		default:
			return nil, rendererConfiguration()
		}
		resolved, resolveErr := filepath.EvalSymlinks(path)
		if resolveErr != nil || (!strings.HasPrefix(resolved, "/opt/homebrew/Cellar/") && !strings.HasPrefix(resolved, "/usr/local/Cellar/") && filepath.Dir(resolved) != "/usr/bin" && filepath.Dir(resolved) != "/opt/homebrew/bin" && filepath.Dir(resolved) != "/usr/local/bin") {
			return nil, rendererConfiguration()
		}
		*tool.path = resolved
	}
	return tools, nil
}

// Preflight resolves tools once and verifies sandbox enforcement before any attempt.
func (p *PopplerRenderer) Preflight(ctx context.Context) error {
	if p.tools != nil {
		return nil
	}
	tools, err := discoverTools(runtime.GOOS, exec.LookPath, os.Stat)
	if err != nil {
		return err
	}
	dir, err := os.MkdirTemp("", "trachuan-ocr-probe-")
	if err != nil {
		return rendererConfiguration()
	}
	defer func() { _ = os.RemoveAll(dir) }()
	probe, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if _, err = tools.command(probe, dir, tools.info, []string{"-v"}, io.Discard); err != nil {
		return rendererConfiguration()
	}
	p.tools = tools
	return nil
}

func sandboxProfile(tool, dir string) string {
	quote := strconv.Quote
	var b strings.Builder
	b.WriteString("(version 1)\n(deny default)\n(deny network*)\n(allow process-fork)\n(allow sysctl-read)\n")
	fmt.Fprintf(&b, "(allow process-exec (literal %s))\n", quote(tool))
	fmt.Fprintf(&b, "(allow file-read* (literal %s) (literal %s) (subpath %s))\n", quote(tool), quote(filepath.Join(dir, "input.pdf")), quote(dir))
	b.WriteString(`(allow file-read*
 (subpath "/System/Library") (subpath "/usr/lib") (subpath "/usr/share")
 (subpath "/Library/Apple/System/Library") (subpath "/private/var/db/dyld")
 (subpath "/opt/homebrew/Cellar") (subpath "/opt/homebrew/opt")
 (subpath "/usr/local/Cellar") (subpath "/usr/local/opt")
 (subpath "/private/etc/fonts") (subpath "/private/var/db/timezone")
 (literal "/") (literal "/private/etc/localtime") (literal "/dev/null")
 (literal "/dev/random") (literal "/dev/urandom"))
`)
	fmt.Fprintf(&b, "(allow file-write* (subpath %s))\n", quote(dir))
	return b.String()
}

func (t *popplerTools) command(ctx context.Context, dir, tool string, args []string, out io.Writer) (RenderStats, error) {
	argv := append([]string{"-p", sandboxProfile(tool, dir), tool}, args...)
	return runCommand(ctx, dir, t.sandbox, argv, out)
}
