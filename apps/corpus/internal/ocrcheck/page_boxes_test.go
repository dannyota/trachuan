package ocrcheck

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"testing"
)

func TestRenderer_BoundsBothPageBoxes(t *testing.T) {
	for _, tc := range []struct {
		name, media, crop string
		scale             bool
	}{
		{"huge media small crop", "0 0 14400 14400", "0 0 72 72", true},
		{"huge crop small media", "0 0 72 72", "0 0 14400 14400", true},
		{"translated boxes", "-14400 -14400 0 0", "10 20 82 92", true},
		{"ordinary scan", "0 0 595 842", "0 0 595 842", false},
		{"pixel boundary", "0 0 1440 1440", "0 0 72 72", false},
		{"above pixel boundary", "0 0 1440.01 1440", "0 0 72 72", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			called := false
			r := PopplerRenderer{tools: &popplerTools{info: "pdfinfo", raster: "pdftoppm"}, command: func(_ context.Context, dir, tool string, args []string, out io.Writer) (RenderStats, error) {
				if tool == "pdfinfo" {
					_, _ = io.WriteString(out, "Pages: 1\nPage 1 size: 72 x 72 pts\nPage 1 MediaBox: "+tc.media+"\nPage 1 CropBox: "+tc.crop+"\n")
					return RenderStats{}, nil
				}
				called = true
				if !slices.Contains(args, "-cropbox") {
					t.Fatal("raster did not select CropBox")
				}
				dpiIndex := slices.Index(args, "-r")
				dpi, parseErr := strconv.ParseFloat(args[dpiIndex+1], 64)
				if parseErr != nil || dpi <= 0 || dpi > 150 {
					t.Fatalf("invalid raster dpi: %v", args)
				}
				if tc.scale && dpi >= 150 {
					t.Fatal("large box did not lower raster resolution")
				}
				if !tc.scale && dpi != 150 {
					t.Fatal("ordinary page unnecessarily lowered resolution")
				}
				scaleIndex := slices.Index(args, "-scale-to")
				if scaleIndex < 0 {
					t.Fatal("raster side cap missing")
				}
				cap, parseErr := strconv.Atoi(args[scaleIndex+1])
				if parseErr != nil || cap < 1 || cap > 3000 {
					t.Fatalf("invalid raster cap: %v", args)
				}
				return RenderStats{}, os.WriteFile(filepath.Join(dir, "page.png"), pagePNG(t, 1, 1), 0600)
			}}
			if _, err := r.Render(context.Background(), smallPDF(), maxRequestBytes); err != nil {
				t.Fatal(err)
			}
			if !called {
				t.Fatal("raster not invoked")
			}
		})
	}
}
func TestRenderer_RequiresValidPageBoxes(t *testing.T) {
	for _, boxes := range []string{
		"", "Page 1 MediaBox: 0 0 72 72\n", "Page 1 MediaBox: 0 0 NaN 72\nPage 1 CropBox: 0 0 72 72\n",
		"Page 1 MediaBox: 0 0 72 72\nPage 1 CropBox: 72 0 0 72\n",
		"Page 1 MediaBox: 0 0 72 72\nPage 1 CropBox: 0 0 72 72\nPage 1 CropBox: 0 0 10 10\n",
	} {
		r := PopplerRenderer{tools: &popplerTools{info: "pdfinfo", raster: "pdftoppm"}, command: func(_ context.Context, _, tool string, _ []string, out io.Writer) (RenderStats, error) {
			if tool != "pdfinfo" {
				t.Fatal("raster allocated without valid boxes")
			}
			_, _ = io.WriteString(out, "Pages: 1\nPage 1 size: 72 x 72 pts\n"+boxes)
			return RenderStats{}, nil
		}}
		if _, err := r.Render(context.Background(), smallPDF(), maxRequestBytes); err == nil {
			t.Fatal("invalid boxes accepted")
		}
	}
}

func TestRenderer_ScalesFromLargerBox(t *testing.T) {
	sizes, err := parsePageSizes([]byte("Pages: 1\nPage 1 size: 72 x 72 pts\nPage 1 MediaBox: 0 0 14400 14400\nPage 1 CropBox: 0 0 72 72\n"))
	if err != nil {
		t.Fatal(err)
	}
	args := rasterArgs(1, sizes[0])
	index := slices.Index(args, "-r")
	dpi, err := strconv.ParseFloat(args[index+1], 64)
	if err != nil || dpi > 15 || dpi <= 0 || args[slices.Index(args, "-scale-to")+1] != "15" {
		t.Fatalf("scale must derive from 14400-point box: args=%v", args)
	}
}
