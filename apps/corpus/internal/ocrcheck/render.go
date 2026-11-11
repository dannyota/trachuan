package ocrcheck

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	maxPages        = 500
	maxImageBytes   = 10_000_000
	maxRequestBytes = 100_000_000
	maxImageSide    = 3000
)

// RenderStats contains only subprocess metadata, never stderr content.
type RenderStats struct {
	ExitCode    int   `json:"exit_code"`
	StderrBytes int64 `json:"stderr_bytes"`
}

// Rendered holds ordered PNG pages and safe process diagnostics.
type Rendered struct {
	Pages [][]byte
	Stats RenderStats
}

// Renderer renders a verified PDF without consulting its embedded text layer.
type Renderer interface {
	Render(context.Context, []byte) (Rendered, error)
}

// PopplerRenderer renders 150 dpi grayscale PNG pages in an isolated directory.
type PopplerRenderer struct {
	run func(context.Context, string) (RenderStats, error)
}

type renderError string

func (e renderError) Error() string { return string(e) }

const renderLimit renderError = "over_check_limit"
const renderFailed renderError = "render_failed"

// Render removes all intermediate files, including after cancellation or failure.
func (p PopplerRenderer) Render(ctx context.Context, pdf []byte) (Rendered, error) {
	result := Rendered{Stats: RenderStats{ExitCode: -1}}
	dir, err := os.MkdirTemp("", "trachuan-ocr-render-")
	if err != nil {
		return result, renderFailed
	}
	defer func() { _ = os.RemoveAll(dir) }()
	if err = os.WriteFile(filepath.Join(dir, "input.pdf"), pdf, 0600); err != nil {
		return result, renderFailed
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	run := p.run
	if run == nil {
		run = runPoppler
	}
	result.Stats, err = run(ctx, dir)
	if err != nil || ctx.Err() != nil || result.Stats.ExitCode != 0 {
		return result, renderFailed
	}
	files, err := filepath.Glob(filepath.Join(dir, "page-*.png"))
	if err != nil || len(files) == 0 {
		return result, renderFailed
	}
	if len(files) > maxPages {
		return result, renderLimit
	}
	// Sort by page number even when a renderer does not zero-pad names.
	number := func(path string) int {
		v, _ := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(filepath.Base(path), "page-"), ".png"))
		return v
	}
	sort.Slice(files, func(i, j int) bool { return number(files[i]) < number(files[j]) })
	total := 0
	for i, path := range files {
		if ctx.Err() != nil {
			return result, renderFailed
		}
		if number(path) != i+1 {
			return result, renderFailed
		}
		f, openErr := os.Open(path)
		if openErr != nil {
			return result, renderFailed
		}
		data, readErr := io.ReadAll(io.LimitReader(f, maxImageBytes))
		_ = f.Close()
		if readErr != nil {
			return result, renderFailed
		}
		if len(data) >= maxImageBytes {
			return result, renderLimit
		}
		if reason := validateImage(data); reason != "" {
			return result, renderError(reason)
		}
		// Poppler can encode -gray PNG output as RGB. Fix the actual PNG color type.
		decoded, decodeErr := png.Decode(bytes.NewReader(data))
		if decodeErr != nil {
			return result, renderFailed
		}
		gray := image.NewGray(decoded.Bounds())
		draw.Draw(gray, gray.Bounds(), decoded, decoded.Bounds().Min, draw.Src)
		var encoded bytes.Buffer
		if encodeErr := png.Encode(&encoded, gray); encodeErr != nil {
			return result, renderFailed
		}
		data = encoded.Bytes()
		if ctx.Err() != nil {
			return result, renderFailed
		}
		total += ((len(data) + 2) / 3) * 4
		if total >= maxRequestBytes {
			return result, renderLimit
		}
		if reason := validateImage(data); reason != "" {
			return result, renderError(reason)
		}
		result.Pages = append(result.Pages, data)
	}
	return result, nil
}

type byteCounter struct{ n int64 }

func (b *byteCounter) Write(p []byte) (int, error) { b.n += int64(len(p)); return len(p), nil }

func runPoppler(ctx context.Context, dir string) (RenderStats, error) {
	// Local filenames and a minimal environment keep credentials out of the child.
	// pdftoppm resolves no URLs and needs no network service.
	cmd := exec.CommandContext(ctx, "pdftoppm", "-r", "150", "-gray", "-png", "input.pdf", "page")
	cmd.Dir = dir
	cmd.Env = []string{"LC_ALL=C", "HOME=" + dir}
	var stderr byteCounter
	cmd.Stderr = &stderr
	cmd.Stdout = io.Discard
	err := cmd.Run()
	stats := RenderStats{ExitCode: -1, StderrBytes: stderr.n}
	if cmd.ProcessState != nil {
		stats.ExitCode = cmd.ProcessState.ExitCode()
	}
	if err != nil {
		return stats, fmt.Errorf("render subprocess: %w", renderFailed)
	}
	return stats, nil
}

func validateImage(data []byte) string {
	if len(data) >= maxImageBytes {
		return "over_check_limit"
	}
	config, err := png.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return "render_failed"
	}
	if config.Width > maxImageSide || config.Height > maxImageSide {
		return "over_check_limit"
	}
	return ""
}
