package ocrcheck

import (
	"context"
	"errors"
	"image/jpeg"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"time"
)

// RenderTooLarge reports that rendering exceeded the scratch ceiling.
const RenderTooLarge renderError = "render_too_large"

func jpegRenderTimeout(pages int) time.Duration {
	return 30*time.Second + time.Duration(pages)*2*time.Second
}

// RenderJPEG renders ordered grayscale JPEG files and returns a scratch cleanup function.
func (p PopplerRenderer) RenderJPEG(ctx context.Context, pdf []byte, pages int) (paths []string, cleanup func(), resultErr error) {
	if pages < 1 || pages > maxPages || len(pdf) > 50_000_000 {
		return nil, nil, renderFailed
	}
	if err := p.Preflight(ctx); err != nil {
		return nil, nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, jpegRenderTimeout(pages))
	defer cancel()
	dir, err := os.MkdirTemp("", "trachuan-ocr-jpeg-")
	if err != nil {
		return nil, nil, renderFailed
	}
	cleanup = func() { _ = os.RemoveAll(dir) }
	defer func() {
		if resultErr != nil {
			cleanup()
			cleanup = nil
		}
	}()
	realDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return nil, cleanup, renderFailed
	}
	dir = realDir
	if err = os.WriteFile(filepath.Join(dir, "input.pdf"), pdf, 0600); err != nil {
		return nil, cleanup, renderFailed
	}
	command := p.command
	if command == nil {
		command = p.tools.command
	}
	run := func(tool string, args []string, out io.Writer) error {
		if ctx.Err() != nil {
			return renderFailed
		}
		if scratchErr := checkScratch(dir); scratchErr != nil {
			if errors.Is(scratchErr, renderLimit) {
				return RenderTooLarge
			}
			return renderFailed
		}
		stats, commandErr := command(ctx, dir, tool, args, out)
		if limit := checkScratch(dir); errors.Is(limit, renderLimit) || errors.Is(commandErr, renderLimit) {
			return RenderTooLarge
		} else if limit != nil {
			return renderFailed
		}
		if commandErr != nil || stats.ExitCode != 0 || ctx.Err() != nil {
			return renderFailed
		}
		return nil
	}
	var metadata metadataBuffer
	if err = run(p.tools.info, []string{"-f", "1", "-l", "500", "-box", "input.pdf"}, &metadata); err != nil {
		return nil, cleanup, err
	}
	sizes, err := parsePageSizes(metadata.Bytes())
	if err != nil || len(sizes) != pages {
		return nil, cleanup, renderFailed
	}
	for first := 0; first < pages; {
		base := rasterArgsDPI(1, sizes[first], 200)
		last := first + 1
		for last < pages && reflect.DeepEqual(base[4:], rasterArgsDPI(1, sizes[last], 200)[4:]) {
			last++
		}
		args := []string{"-jpeg", "-jpegopt", "quality=90", "-gray", "-f", strconv.Itoa(first + 1), "-l", strconv.Itoa(last), "-r", base[5], "-cropbox", "-scale-to", base[11], "input.pdf", "page"}
		if err = run(p.tools.raster, args, io.Discard); err != nil {
			return nil, cleanup, err
		}
		first = last
	}
	paths, err = jpegPages(ctx, dir, pages, p.validateJPEG)
	if err != nil {
		return nil, cleanup, err
	}
	if ctx.Err() != nil {
		return nil, cleanup, renderFailed
	}
	return paths, cleanup, nil
}

func jpegPages(ctx context.Context, dir string, pages int, validate func(string) error) ([]string, error) {
	if validate == nil {
		validate = validateJPEG
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, renderFailed
	}
	paths := make([]string, pages)
	for _, entry := range entries {
		if ctx.Err() != nil {
			return nil, renderFailed
		}
		name := entry.Name()
		if !strings.HasPrefix(name, "page") {
			continue
		}
		if !strings.HasPrefix(name, "page-") || !strings.HasSuffix(name, ".jpg") || !entry.Type().IsRegular() {
			return nil, renderFailed
		}
		number := strings.TrimSuffix(strings.TrimPrefix(name, "page-"), ".jpg")
		for _, c := range number {
			if c < '0' || c > '9' {
				return nil, renderFailed
			}
		}
		n, err := strconv.Atoi(number)
		if err != nil || n < 1 || n > pages || paths[n-1] != "" {
			return nil, renderFailed
		}
		path := filepath.Join(dir, name)
		validationErr := validate(path)
		if ctx.Err() != nil {
			return nil, renderFailed
		}
		if validationErr != nil {
			return nil, validationErr
		}
		paths[n-1] = path
	}
	for _, path := range paths {
		if path == "" {
			return nil, renderFailed
		}
	}
	if ctx.Err() != nil {
		return nil, renderFailed
	}
	return paths, nil
}

func validateJPEG(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return renderFailed
	}
	info, statErr := f.Stat()
	if statErr != nil || info.Size() > 7_000_000 {
		_ = f.Close()
		if statErr == nil {
			return PageTooLarge
		}
		return renderFailed
	}
	config, configErr := jpeg.DecodeConfig(f)
	// Poppler 26 writes three components even with -gray, so the color model is not checked.
	if configErr != nil || config.Width > maxImageSide || config.Height > maxImageSide {
		_ = f.Close()
		return renderFailed
	}
	if _, seekErr := f.Seek(0, io.SeekStart); seekErr != nil {
		_ = f.Close()
		return renderFailed
	}
	_, decodeErr := jpeg.Decode(f)
	closeErr := f.Close()
	if decodeErr != nil || closeErr != nil {
		return renderFailed
	}
	return nil
}
