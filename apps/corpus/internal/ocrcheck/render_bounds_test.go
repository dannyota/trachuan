package ocrcheck

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestRenderer_BoundsAndDeletesEachPage(t *testing.T) {
	var dir string
	infoCalls, rasterCalls := 0, 0
	renderer := PopplerRenderer{tools: &popplerTools{info: "pdfinfo", raster: "pdftoppm"}, command: func(_ context.Context, work, tool string, args []string, out io.Writer) (RenderStats, error) {
		dir = work
		if tool == "pdfinfo" {
			if rasterCalls != 0 {
				t.Fatal("page sizes read after raster allocation")
			}
			infoCalls++
			_, _ = io.WriteString(out, "Pages: 2\nPage    1 size: 72000 x 720 pts\nPage    2 size: 72 x 72 pts\n")
			return RenderStats{}, nil
		}
		if infoCalls != 1 {
			t.Fatal("raster invoked before pdfinfo")
		}
		if _, err := os.Stat(filepath.Join(work, "page.png")); !os.IsNotExist(err) {
			t.Fatal("previous page not removed before next allocation")
		}
		rasterCalls++
		want := []string{"-f", fmt.Sprint(rasterCalls), "-l", fmt.Sprint(rasterCalls), "-r", "150", "-gray", "-png", "-singlefile"}
		if rasterCalls == 1 {
			want = append(want, "-scale-to", "3000")
		}
		want = append(want, "input.pdf", "page")
		if !reflect.DeepEqual(args, want) {
			t.Fatalf("unbounded raster args=%v want=%v", args, want)
		}
		if err := os.WriteFile(filepath.Join(work, "page.png"), pagePNG(t, rasterCalls, 1), 0600); err != nil {
			t.Fatal(err)
		}
		return RenderStats{}, nil
	}}
	result, err := renderer.Render(context.Background(), smallPDF(), maxRequestBytes)
	if err != nil || len(result.Pages) != 2 || rasterCalls != 2 {
		t.Fatalf("pages=%d calls=%d error=%v", len(result.Pages), rasterCalls, err)
	}
	if _, err = os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("scratch retained")
	}
}
func TestRenderer_PageLimitBeforeRaster(t *testing.T) {
	raster := 0
	renderer := PopplerRenderer{tools: &popplerTools{info: "pdfinfo", raster: "pdftoppm"}, command: func(_ context.Context, _, tool string, _ []string, out io.Writer) (RenderStats, error) {
		if tool == "pdfinfo" {
			_, _ = io.WriteString(out, "Pages: 501\n")
		} else {
			raster++
		}
		return RenderStats{}, nil
	}}
	_, err := renderer.Render(context.Background(), smallPDF(), maxRequestBytes)
	if err == nil || err.Error() != "over_check_limit" || raster != 0 {
		t.Fatalf("raster calls=%d error=%v", raster, err)
	}
}
func TestRenderer_ScratchLimit(t *testing.T) {
	renderer := PopplerRenderer{tools: &popplerTools{info: "pdfinfo", raster: "pdftoppm"}, command: func(_ context.Context, dir, tool string, _ []string, out io.Writer) (RenderStats, error) {
		if tool == "pdfinfo" {
			_, _ = io.WriteString(out, "Pages: 1\nPage    1 size: 72 x 72 pts\n")
			return RenderStats{}, nil
		}
		f, err := os.Create(filepath.Join(dir, "excess"))
		if err != nil {
			t.Fatal(err)
		}
		if err = f.Truncate(256_000_000); err != nil {
			t.Fatal(err)
		}
		_ = f.Close()
		if err = os.WriteFile(filepath.Join(dir, "page.png"), pagePNG(t, 1, 1), 0600); err != nil {
			t.Fatal(err)
		}
		return RenderStats{}, nil
	}}
	_, err := renderer.Render(context.Background(), smallPDF(), maxRequestBytes)
	if err == nil || !strings.Contains(err.Error(), "over_check_limit") {
		t.Fatalf("scratch bound not enforced: %v", err)
	}
}

func TestRenderer_RequestBudgetStopsBeforeNextRaster(t *testing.T) {
	raster := 0
	renderer := PopplerRenderer{tools: &popplerTools{info: "pdfinfo", raster: "pdftoppm"}, command: func(_ context.Context, dir, tool string, _ []string, out io.Writer) (RenderStats, error) {
		if tool == "pdfinfo" {
			_, _ = io.WriteString(out, "Pages: 2\nPage    1 size: 72 x 72 pts\nPage    2 size: 72 x 72 pts\n")
			return RenderStats{}, nil
		}
		raster++
		if err := os.WriteFile(filepath.Join(dir, "page.png"), pagePNG(t, 1, 1), 0600); err != nil {
			t.Fatal(err)
		}
		return RenderStats{}, nil
	}}
	_, err := renderer.Render(context.Background(), smallPDF(), 1)
	if err == nil || err.Error() != "over_check_limit" || raster > 1 {
		t.Fatalf("request budget ignored: calls=%d error=%v", raster, err)
	}
}
