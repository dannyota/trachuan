package ocrcheck

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

const sandboxExecutable = "/usr/bin/sandbox-exec"

type popplerTools struct {
	info, raster, sandbox string
	access                runtimeAccess
}

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
	access, err := resolveRuntime([]string{tools.info, tools.raster})
	if err != nil {
		return rendererConfiguration()
	}
	tools.access = access
	dir, err := os.MkdirTemp("", "trachuan-ocr-probe-")
	if err != nil {
		return rendererConfiguration()
	}
	defer func() { _ = os.RemoveAll(dir) }()
	probe, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	for _, tool := range []string{tools.info, tools.raster} {
		if _, err = tools.command(probe, dir, tool, []string{"-v"}, io.Discard); err != nil {
			return rendererConfiguration()
		}
	}
	p.tools = tools
	return nil
}

func sandboxProfile(tool, dir string, permissions ...runtimeAccess) string {
	quote := strconv.Quote
	var b strings.Builder
	b.WriteString("(version 1)\n(deny default)\n(deny network*)\n(allow process-fork)\n(allow sysctl-read)\n")
	fmt.Fprintf(&b, "(allow process-exec (literal %s))\n", quote(tool))
	fmt.Fprintf(&b, "(allow file-read* (literal %s) (literal %s) (subpath %s))\n", quote(tool), quote(filepath.Join(dir, "input.pdf")), quote(dir))
	b.WriteString("(allow file-read* (literal \"/\") (literal \"/dev/null\") (literal \"/dev/random\") (literal \"/dev/urandom\"))\n")
	for _, access := range permissions {
		// dyld resolves each library through its parent directories and the
		// symlinks inside them: it needs their metadata, never their contents.
		// Metadata is granted for each library's own directory and, as single
		// entries, for the directories above it.
		parents := map[string]bool{}
		ancestors := map[string]bool{}
		for _, path := range access.files {
			fmt.Fprintf(&b, "(allow file-read* (literal %s))\n", quote(path))
			parent := filepath.Dir(path)
			parents[parent] = true
			for up := filepath.Dir(parent); up != "/" && up != "." && !ancestors[up]; up = filepath.Dir(up) {
				ancestors[up] = true
			}
		}
		for _, group := range []struct {
			set  map[string]bool
			rule string
		}{{parents, "subpath"}, {ancestors, "literal"}} {
			names := make([]string, 0, len(group.set))
			for name := range group.set {
				names = append(names, name)
			}
			sort.Strings(names)
			for _, name := range names {
				fmt.Fprintf(&b, "(allow file-read-metadata (%s %s))\n", group.rule, quote(name))
			}
		}
		for _, path := range access.dirs {
			fmt.Fprintf(&b, "(allow file-read* (subpath %s))\n", quote(path))
		}
	}
	fmt.Fprintf(&b, "(allow file-write* (subpath %s))\n", quote(dir))
	return b.String()
}

func (t *popplerTools) command(ctx context.Context, dir, tool string, args []string, out io.Writer) (RenderStats, error) {
	argv := append([]string{"-p", sandboxProfile(tool, dir, t.access), tool}, args...)
	return runCommand(ctx, dir, t.sandbox, argv, out)
}
