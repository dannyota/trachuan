package ocrcheck

import (
	"bytes"
	"context"
	"fmt"
	"image/jpeg"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

func mergedPages(t *testing.T, pages [][]byte) []byte {
	t.Helper()
	inputs := make([]io.ReadSeeker, len(pages))
	for i, page := range pages {
		inputs[i] = bytes.NewReader(page)
	}
	var out bytes.Buffer
	conf := model.NewStatelessConfiguration()
	conf.Offline = true
	if err := api.MergeRaw(context.Background(), inputs, &out, false, conf); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func TestJPEGRender_Native(t *testing.T) {
	r := nativeSandbox(t)
	help, _ := exec.Command(r.tools.raster, "-h").CombinedOutput()
	for _, flag := range []string{"-jpeg ", "-jpegopt ", "-gray ", "-cropbox ", "-scale-to ", "-f ", "-l "} {
		if !strings.Contains(string(help), flag) {
			t.Fatalf("installed pdftoppm lacks %s", flag)
		}
	}
	for _, mixed := range []bool{false, true} {
		t.Run(fmt.Sprint(mixed), func(t *testing.T) {
			pages := make([][]byte, 12)
			for i := range pages {
				pages[i] = smallPDF()
				if mixed && i >= 6 {
					pages[i] = pdfWithBoxes("0 0 14400 14400", "0 0 72 72")
				}
			}
			pdf := mergedPages(t, pages)
			count, err := CountPages(context.Background(), pdf)
			if err != nil || count != 12 {
				t.Fatalf("count=%d err=%v", count, err)
			}
			calls := 0
			r.command = func(ctx context.Context, dir, tool string, args []string, out io.Writer) (RenderStats, error) {
				if tool == r.tools.raster {
					calls++
				}
				return r.tools.command(ctx, dir, tool, args, out)
			}
			paths, cleanup, err := r.RenderJPEG(context.Background(), pdf, count)
			if err != nil {
				t.Fatal(err)
			}
			defer cleanup()
			want := 1
			if mixed {
				want = 2
			}
			if len(paths) != 12 || calls != want {
				t.Fatalf("pages=%d calls=%d", len(paths), calls)
			}
			for i, path := range paths {
				name := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(path), "page-"), ".jpg")
				n, parseErr := strconv.Atoi(name)
				if parseErr != nil || n != i+1 {
					t.Fatalf("order: %v", paths)
				}
				f, err := os.Open(path)
				if err != nil {
					t.Fatal(err)
				}
				img, err := jpeg.Decode(f)
				_ = f.Close()
				if err != nil || img.Bounds().Dx() > 3000 || img.Bounds().Dy() > 3000 {
					t.Fatalf("page %d invalid: %v", i+1, err)
				}
			}
		})
	}
}

func TestJPEGRender_FixturePageCount(t *testing.T) {
	pages := make([][]byte, 12)
	for i := range pages {
		pages[i] = smallPDF()
	}
	pdf := mergedPages(t, pages)
	count, err := CountPages(context.Background(), pdf)
	if err != nil || count != 12 {
		t.Fatalf("count=%d err=%v", count, err)
	}
}
