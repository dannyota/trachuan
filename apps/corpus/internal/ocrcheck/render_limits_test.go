package ocrcheck

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestRenderer_EnforcesPageAndTotalBytesAsProduced(t *testing.T) {
	page := pagePNG(t, 1, 1)
	for _, tc := range []struct {
		name                           string
		pageBytes, totalBytes, renders int
		reason                         string
	}{
		{"page", len(page) - 1, 1000, 1, "page_too_large"},
		{"total", len(page), len(page) + 1, 2, "over_check_limit"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			renders := 0
			renderer := PopplerRenderer{tools: &popplerTools{info: "info", raster: "raster"}, command: func(_ context.Context, dir, tool string, _ []string, out io.Writer) (RenderStats, error) {
				if tool == "info" {
					_, _ = fmt.Fprint(out, "Pages: 2\nPage 1 MediaBox: 0 0 72 72\nPage 1 CropBox: 0 0 72 72\nPage 2 MediaBox: 0 0 72 72\nPage 2 CropBox: 0 0 72 72\n")
					return RenderStats{}, nil
				}
				renders++
				return RenderStats{}, os.WriteFile(filepath.Join(dir, "page.png"), page, 0600)
			}}
			_, err := renderer.RenderLimited(context.Background(), smallPDF(), RenderLimits{DPI: 200, PageBytes: tc.pageBytes, TotalBytes: tc.totalBytes})
			if err == nil || err.Error() != tc.reason || renders != tc.renders {
				t.Fatalf("renders=%d err=%v", renders, err)
			}
		})
	}
}
