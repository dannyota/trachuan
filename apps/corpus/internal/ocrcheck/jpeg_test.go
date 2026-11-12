package ocrcheck

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestJPEGRender_Completion(t *testing.T) {
	for _, mode := range []string{"complete", "missing", "extra", "truncated", "exit"} {
		t.Run(mode, func(t *testing.T) {
			calls := 0
			renderer := PopplerRenderer{tools: &popplerTools{info: "info", raster: "raster"}, command: func(_ context.Context, dir, tool string, _ []string, out io.Writer) (RenderStats, error) {
				if tool == "info" {
					_, _ = fmt.Fprintln(out, "Pages: 2\nPage 1 MediaBox: 0 0 72 72\nPage 1 CropBox: 0 0 72 72\nPage 2 MediaBox: 0 0 72 72\nPage 2 CropBox: 0 0 72 72")
					return RenderStats{}, nil
				}
				calls++
				var b bytes.Buffer
				if err := jpeg.Encode(&b, image.NewGray(image.Rect(0, 0, 2, 2)), nil); err != nil {
					t.Fatal(err)
				}
				for i := 1; i <= 2; i++ {
					if mode == "missing" && i == 2 {
						continue
					}
					data := b.Bytes()
					if mode == "truncated" && i == 2 {
						data = data[:len(data)/2]
					}
					if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("page-%02d.jpg", i)), data, 0600); err != nil {
						t.Fatal(err)
					}
				}
				if mode == "extra" {
					if err := os.WriteFile(filepath.Join(dir, "page-03.jpg"), b.Bytes(), 0600); err != nil {
						t.Fatal(err)
					}
				}
				if mode == "exit" {
					return RenderStats{ExitCode: 1}, nil
				}
				return RenderStats{}, nil
			}}
			paths, cleanup, err := renderer.RenderJPEG(context.Background(), smallPDF(), 2)
			if cleanup != nil {
				defer cleanup()
			}
			if mode == "complete" {
				if err != nil || len(paths) != 2 || calls != 1 {
					t.Fatalf("paths=%v calls=%d err=%v", paths, calls, err)
				}
			} else if err == nil {
				t.Fatal("accepted incomplete render")
			}
		})
	}
}

func TestJPEGRender_Deadline(t *testing.T) {
	if jpegRenderTimeout(1) != 32*time.Second || jpegRenderTimeout(12) != 54*time.Second {
		t.Fatal("incorrect document deadline")
	}
}

func TestJPEGRender_GroupsAndScratchLimit(t *testing.T) {
	t.Run("groups", func(t *testing.T) {
		var calls [][]string
		r := PopplerRenderer{tools: &popplerTools{info: "info", raster: "raster"}, command: func(_ context.Context, dir, tool string, args []string, out io.Writer) (RenderStats, error) {
			if tool == "info" {
				_, _ = fmt.Fprintln(out, "Pages: 3\nPage 1 MediaBox: 0 0 72 72\nPage 1 CropBox: 0 0 72 72\nPage 2 MediaBox: 0 0 72 72\nPage 2 CropBox: 0 0 72 72\nPage 3 MediaBox: 0 0 14400 14400\nPage 3 CropBox: 0 0 72 72")
				return RenderStats{}, nil
			}
			calls = append(calls, append([]string(nil), args...))
			var b bytes.Buffer
			if err := jpeg.Encode(&b, image.NewGray(image.Rect(0, 0, 2, 2)), nil); err != nil {
				t.Fatal(err)
			}
			first, _ := strconv.Atoi(args[5])
			last, _ := strconv.Atoi(args[7])
			for i := first; i <= last; i++ {
				if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("page-%d.jpg", i)), b.Bytes(), 0600); err != nil {
					t.Fatal(err)
				}
			}
			return RenderStats{}, nil
		}}
		paths, cleanup, err := r.RenderJPEG(context.Background(), smallPDF(), 3)
		if err != nil {
			t.Fatal(err)
		}
		defer cleanup()
		want := [][]string{{"-jpeg", "-jpegopt", "quality=90", "-gray", "-f", "1", "-l", "2", "-r", "200", "-cropbox", "-scale-to", "200", "input.pdf", "page"}, {"-jpeg", "-jpegopt", "quality=90", "-gray", "-f", "3", "-l", "3", "-r", "15", "-cropbox", "-scale-to", "15", "input.pdf", "page"}}
		if !reflect.DeepEqual(calls, want) || len(paths) != 3 {
			t.Fatalf("calls=%v paths=%v", calls, paths)
		}
	})
	t.Run("scratch", func(t *testing.T) {
		script := filepath.Join(t.TempDir(), "writer")
		if err := os.WriteFile(script, []byte("#!/bin/sh\n/bin/dd if=/dev/zero of=excess bs=1 seek=256000000 count=1 2>/dev/null\nwhile :; do printf x >> growing; done\n"), 0700); err != nil {
			t.Fatal(err)
		}
		var work string
		r := PopplerRenderer{tools: &popplerTools{info: "info", raster: "raster"}, command: func(ctx context.Context, dir, tool string, _ []string, out io.Writer) (RenderStats, error) {
			work = dir
			if tool == "info" {
				_, _ = fmt.Fprintln(out, "Pages: 1\nPage 1 MediaBox: 0 0 72 72\nPage 1 CropBox: 0 0 72 72")
				return RenderStats{}, nil
			}
			return runCommand(ctx, dir, script, nil, out)
		}}
		_, cleanup, err := r.RenderJPEG(context.Background(), smallPDF(), 1)
		if cleanup != nil {
			cleanup()
		}
		if err != RenderTooLarge {
			t.Fatalf("error=%v", err)
		}
		if _, err := os.Stat(work); !os.IsNotExist(err) {
			t.Fatal("scratch was not removed")
		}
	})
}

func TestJPEGPages_OrderAndComponents(t *testing.T) {
	dir := t.TempDir()
	var gray, rgb bytes.Buffer
	if err := jpeg.Encode(&gray, image.NewGray(image.Rect(0, 0, 2, 2)), nil); err != nil {
		t.Fatal(err)
	}
	if err := jpeg.Encode(&rgb, image.NewRGBA(image.Rect(0, 0, 2, 2)), nil); err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 12; i++ {
		name := fmt.Sprintf("page-%d.jpg", i)
		if i%2 == 0 {
			name = fmt.Sprintf("page-%03d.jpg", i)
		}
		if err := os.WriteFile(filepath.Join(dir, name), gray.Bytes(), 0600); err != nil {
			t.Fatal(err)
		}
	}
	paths, err := jpegPages(dir, 12)
	if err != nil {
		t.Fatal(err)
	}
	for i, path := range paths {
		n, err := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(filepath.Base(path), "page-"), ".jpg"))
		if err != nil || n != i+1 {
			t.Fatalf("incorrect order %v", paths)
		}
	}
	if err := os.WriteFile(paths[0], rgb.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	// Poppler writes three components for gray content; such a page is valid.
	if _, err := jpegPages(dir, 12); err != nil {
		t.Fatalf("rejected three-component JPEG: %v", err)
	}
}
