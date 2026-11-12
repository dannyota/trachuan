package ocrcheck

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/anthropics/anthropic-sdk-go/v2"
)

const (
	maxPages              = 500
	maxImageBytes         = 10_000_000
	maxRequestBytes       = 100_000_000
	maxImageSide          = 3000
	maxScratchBytes int64 = 256_000_000
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

// Renderer renders verified PDF bytes within the remaining encoded request budget.
type Renderer interface {
	Preflight(context.Context) error
	Render(context.Context, []byte, int) (Rendered, error)
}

type renderCommand func(context.Context, string, string, []string, io.Writer) (RenderStats, error)

// PopplerRenderer renders bounded 150 dpi grayscale PNG pages in a macOS sandbox.
type PopplerRenderer struct {
	tools        *popplerTools
	command      renderCommand
	validateJPEG func(string) error
}

type renderError string

func (e renderError) Error() string { return string(e) }

const renderLimit renderError = "over_check_limit"
const renderFailed renderError = "render_failed"

// Render removes every page before allocating the next and always removes scratch.
func (p PopplerRenderer) Render(ctx context.Context, pdf []byte, allowance int) (result Rendered, renderErr error) {
	return p.RenderDPI(ctx, pdf, allowance, 150)
}

// RenderLimits bounds raw PNG payloads for callers that send pages separately.
type RenderLimits struct {
	DPI, PageBytes, TotalBytes int
}

// PageTooLarge reports a page that exceeds the caller's raw PNG limit.
const PageTooLarge renderError = "page_too_large"

// RenderLimited checks each page as produced and caps retained PNG bytes.
func (p PopplerRenderer) RenderLimited(ctx context.Context, pdf []byte, limits RenderLimits) (Rendered, error) {
	if limits.PageBytes < 1 || limits.TotalBytes < 1 {
		return Rendered{}, renderFailed
	}
	result, err := p.render(ctx, pdf, 0, limits.DPI, limits.PageBytes, limits.TotalBytes)
	if err != nil {
		result.Pages = nil
	}
	return result, err
}

// RenderDPI uses the contained renderer at the requested resolution and raster cap.
func (p PopplerRenderer) RenderDPI(ctx context.Context, pdf []byte, allowance, dpi int) (Rendered, error) {
	return p.render(ctx, pdf, allowance, dpi, 0, 0)
}

func (p PopplerRenderer) render(ctx context.Context, pdf []byte, allowance, dpi, pageLimit, totalLimit int) (result Rendered, renderErr error) {
	if dpi < 1 || dpi > 200 {
		return result, renderFailed
	}
	result.Stats.ExitCode = -1
	if err := p.Preflight(ctx); err != nil {
		return result, err
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	dir, err := os.MkdirTemp("", "trachuan-ocr-render-")
	if err != nil {
		return result, renderFailed
	}
	// Resolve /var and /tmp aliases for sandbox path matching.
	realDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		_ = os.RemoveAll(dir)
		return result, renderFailed
	}
	dir = realDir
	defer func() {
		if removeErr := os.RemoveAll(dir); removeErr != nil {
			result.Pages = nil
			renderErr = renderFailed
		}
	}()
	if len(pdf) > 50_000_000 {
		return result, renderLimit
	}
	if err = os.WriteFile(filepath.Join(dir, "input.pdf"), pdf, 0600); err != nil {
		return result, renderFailed
	}
	command := p.command
	if command == nil {
		command = p.tools.command
	}
	run := func(tool string, args []string, out io.Writer) error {
		if ctx.Err() != nil {
			return renderFailed
		}
		if limitErr := checkScratch(dir); limitErr != nil {
			return limitErr
		}
		stats, runErr := command(ctx, dir, tool, args, out)
		result.Stats.ExitCode = stats.ExitCode
		result.Stats.StderrBytes += stats.StderrBytes
		if limitErr := checkScratch(dir); limitErr != nil {
			return limitErr
		}
		if runErr != nil {
			if errors.Is(runErr, renderLimit) {
				return renderLimit
			}
			return renderFailed
		}
		if ctx.Err() != nil || stats.ExitCode != 0 {
			return renderFailed
		}
		return nil
	}
	var metadata metadataBuffer
	if err = run(p.tools.info, []string{"-f", "1", "-l", "500", "-box", "input.pdf"}, &metadata); err != nil {
		return result, err
	}
	sizes, err := parsePageSizes(metadata.Bytes())
	if err != nil {
		return result, err
	}
	total := 0
	for i, size := range sizes {
		if err = run(p.tools.raster, rasterArgsDPI(i+1, size, dpi), io.Discard); err != nil {
			return result, err
		}
		page, readErr := readRenderedPageLimit(filepath.Join(dir, "page.png"), pageLimit)
		if readErr != nil {
			return result, readErr
		}
		if ctx.Err() != nil {
			return result, renderFailed
		}
		if totalLimit > 0 {
			if len(page) > totalLimit-total {
				return result, renderLimit
			}
			total += len(page)
		} else {
			total += encodedPageSize(i+1, page)
			if total >= allowance {
				return result, renderLimit
			}
		}
		result.Pages = append(result.Pages, page)
	}
	return result, nil
}

func readRenderedPageLimit(path string, pageLimit int) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, renderFailed
	}
	readLimit := int64(maxImageBytes)
	if pageLimit > 0 && pageLimit < maxImageBytes {
		readLimit = int64(pageLimit) + 1
	}
	data, err := io.ReadAll(io.LimitReader(f, readLimit))
	_ = f.Close()
	if err != nil {
		return nil, renderFailed
	}
	if pageLimit > 0 && len(data) > pageLimit {
		return nil, PageTooLarge
	}
	if reason := validateImage(data); reason != "" {
		return nil, renderError(reason)
	}
	decoded, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, renderFailed
	}
	// Poppler may encode -gray output as RGB; force grayscale PNG encoding.
	gray := image.NewGray(decoded.Bounds())
	draw.Draw(gray, gray.Bounds(), decoded, decoded.Bounds().Min, draw.Src)
	var encoded bytes.Buffer
	if err = png.Encode(&encoded, gray); err != nil {
		return nil, renderFailed
	}
	if pageLimit > 0 && encoded.Len() > pageLimit {
		return nil, PageTooLarge
	}
	if reason := validateImage(encoded.Bytes()); reason != "" {
		return nil, renderError(reason)
	}
	if err = os.Remove(path); err != nil {
		return nil, renderFailed
	}
	return encoded.Bytes(), nil
}

func encodedPageSize(n int, page []byte) int {
	blocks := []anthropic.ContentBlockParamUnion{anthropic.NewTextBlock(fmt.Sprintf("Page %d:", n)), anthropic.NewImageBlockBase64("image/png", base64.StdEncoding.EncodeToString(page))}
	data, err := json.Marshal(blocks)
	if err != nil {
		return maxRequestBytes
	}
	// Each pair adds its two blocks and two commas to the existing content array.
	return len(data) - 1
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

type byteCounter struct{ n int64 }

func (b *byteCounter) Write(p []byte) (int, error) { b.n += int64(len(p)); return len(p), nil }
