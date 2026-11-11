package ocrcheck

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type unavailableRenderer struct{ *fakeRenderer }

func (*unavailableRenderer) Preflight(context.Context) error {
	return errors.New("OCR renderer sandbox configuration unavailable")
}

func TestCheck_RequiresSandboxBeforeAttempts(t *testing.T) {
	for _, dry := range []bool{false, true} {
		r, repo, client, docs := fixture(t)
		r.Renderer = &unavailableRenderer{&fakeRenderer{}}
		o := options()
		o.DryRun = dry
		_, err := r.Run(context.Background(), docs, o)
		if err == nil || !strings.Contains(err.Error(), "configuration") || len(repo.attempts) != 0 || client.CountCalls != 0 {
			t.Fatalf("sandbox not required: err=%v attempts=%d counts=%d", err, len(repo.attempts), client.CountCalls)
		}
	}
}
func TestRenderer_RejectsShadowedToolBeforeAttempts(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"pdftoppm", "pdfinfo"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\nexit 7\n"), 0700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir)
	r, repo, _, docs := fixture(t)
	r.Renderer = nil
	_, err := r.Run(context.Background(), docs, options())
	if err == nil || !strings.Contains(err.Error(), "configuration") || len(repo.attempts) != 0 {
		t.Fatalf("shadowed tool accepted: error=%v attempts=%d", err, len(repo.attempts))
	}
}

func TestRenderer_UnsupportedSandboxConfigurations(t *testing.T) {
	info, err := os.Stat(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, goos, path string
		missing          bool
	}{
		{"unsupported OS", "linux", "/usr/bin/pdfinfo", false},
		{"missing sandbox", "darwin", "/usr/bin/pdfinfo", true},
		{"relative tool", "darwin", "bin/pdfinfo", false},
		{"shadowed tool", "darwin", "/tmp/pdfinfo", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := discoverTools(tc.goos, func(string) (string, error) { return tc.path, nil }, func(string) (os.FileInfo, error) {
				if tc.missing {
					return nil, os.ErrNotExist
				}
				return executableInfo{info}, nil
			})
			if err == nil || !strings.Contains(err.Error(), "configuration") {
				t.Fatal("unsafe configuration accepted")
			}
		})
	}
}

type executableInfo struct{ os.FileInfo }

func (executableInfo) Mode() os.FileMode { return 0755 }

func TestRenderer_SandboxProfileRestrictions(t *testing.T) {
	profile := sandboxProfile("/opt/homebrew/bin/pdfinfo", "/private/tmp/ocr-test")
	for _, rule := range []string{`(deny default)`, `(deny network*)`, `(allow process-exec (literal "/opt/homebrew/bin/pdfinfo"))`, `(allow file-write* (subpath "/private/tmp/ocr-test"))`} {
		if !strings.Contains(profile, rule) {
			t.Fatalf("missing sandbox restriction: %s", rule)
		}
	}
	for _, broad := range []string{`(allow default)`, `(allow network`, `(subpath "/")`, `(subpath "/Users")`, `(allow process-exec)`} {
		if strings.Contains(profile, broad) {
			t.Fatalf("sandbox too broad: %s", broad)
		}
	}
}
