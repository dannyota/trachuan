package ocrcheck

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type fakeRenderer struct {
	pages [][]byte
	err   error
	calls int
}

func (f *fakeRenderer) Render(context.Context, []byte) (Rendered, error) {
	f.calls++
	return Rendered{Pages: f.pages, Stats: RenderStats{ExitCode: 0, StderrBytes: 17}}, f.err
}
func pagePNG(t *testing.T, width, height int) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewGray(image.Rect(0, 0, width, height))); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}
func fakePages(t *testing.T, n int) [][]byte {
	t.Helper()
	pages := make([][]byte, n)
	p := pagePNG(t, 1, 1)
	for i := range pages {
		pages[i] = p
	}
	return pages
}
func TestCheck_RenderedLimits(t *testing.T) {
	for _, tc := range []struct {
		name, reason string
		count        int
		pages        func(*testing.T) [][]byte
	}{
		{"count mismatch", "page_count_mismatch", 10, func(t *testing.T) [][]byte { return fakePages(t, 9) }},
		{"page limit", "over_check_limit", 501, func(t *testing.T) [][]byte { return fakePages(t, 501) }},
		{"image size", "over_check_limit", 1, func(*testing.T) [][]byte { return [][]byte{make([]byte, 10_000_000)} }},
		{"image dimensions", "over_check_limit", 1, func(t *testing.T) [][]byte { return [][]byte{pagePNG(t, 3001, 1)} }},
		{"invalid PNG", "render_failed", 1, func(*testing.T) [][]byte { return [][]byte{[]byte("untrusted image")} }},
		{"request size", "over_check_limit", 10, func(t *testing.T) [][]byte {
			p := append(pagePNG(t, 1, 1), make([]byte, 8_000_000)...)
			pages := make([][]byte, 10)
			for i := range pages {
				pages[i] = p
			}
			return pages
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, s, c, d := fixture(t)
			r.CountPages = func(context.Context, []byte) (int, error) { return tc.count, nil }
			r.Renderer = &fakeRenderer{pages: tc.pages(t)}
			if err := run(t, r, d, options()); err != nil {
				t.Fatal(err)
			}
			if c.CountCalls != 0 || c.DirectCalls != 0 || len(s.attempts) != 1 || s.attempts[0].Reason != tc.reason {
				t.Fatalf("reason=%s", s.attempts[0].Reason)
			}
		})
	}
}
func TestCheck_DryRunCountsImages(t *testing.T) {
	r, s, c, d := fixture(t)
	o := options()
	o.DryRun = true
	summary, err := r.Run(context.Background(), d, o)
	if err != nil {
		t.Fatal(err)
	}
	if c.CountCalls != 1 || len(c.Counted[0].Images) != 10 || len(s.attempts) != 0 || summary.Estimated != 1 || len(summary.Documents) != 1 || summary.Documents[0].Pages != 10 {
		t.Fatal("dry run omitted rendered images or wrote an attempt")
	}
}
func TestRenderer_TimeoutAndNonzeroExit(t *testing.T) {
	for _, tc := range []struct {
		name string
		code int
		err  error
	}{
		{"timeout", -1, context.DeadlineExceeded}, {"nonzero", 7, fmt.Errorf("untrusted stderr")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var dir string
			renderer := PopplerRenderer{run: func(ctx context.Context, work string) (RenderStats, error) {
				dir = work
				deadline, ok := ctx.Deadline()
				if !ok || time.Until(deadline) > 2*time.Minute || time.Until(deadline) < 119*time.Second {
					t.Fatal("missing render deadline")
				}
				return RenderStats{ExitCode: tc.code, StderrBytes: 19}, tc.err
			}}
			result, err := renderer.Render(context.Background(), []byte("private PDF"))
			if err == nil || strings.Contains(err.Error(), "untrusted") || result.Stats.ExitCode != tc.code || result.Stats.StderrBytes != 19 {
				t.Fatal("unsafe or missing render diagnostics")
			}
			if _, err = os.Stat(dir); !os.IsNotExist(err) {
				t.Fatal("render directory retained")
			}
		})
	}
}
func TestRenderer_StderrIsCountedOnly(t *testing.T) {
	dir := t.TempDir()
	binary := filepath.Join(dir, "pdftoppm")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\nprintf 'sensitive-renderer-text' >&2\nexit 7\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	result, err := (PopplerRenderer{}).Render(context.Background(), []byte("private PDF"))
	if err == nil || strings.Contains(err.Error(), "sensitive") || result.Stats.ExitCode != 7 || result.Stats.StderrBytes != 23 {
		t.Fatalf("stats=%+v error=%v", result.Stats, err)
	}
}
func smallPDF() []byte {
	var pdf bytes.Buffer
	pdf.WriteString("%PDF-1.7\n")
	offsets := []int{0}
	for i, obj := range []string{"<< /Type /Catalog /Pages 2 0 R >>", "<< /Type /Pages /Kids [3 0 R] /Count 1 >>", "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 72 72] /Resources << >> >>"} {
		offsets = append(offsets, pdf.Len())
		fmt.Fprintf(&pdf, "%d 0 obj\n%s\nendobj\n", i+1, obj)
	}
	xref := pdf.Len()
	pdf.WriteString("xref\n0 4\n0000000000 65535 f \n")
	for _, offset := range offsets[1:] {
		fmt.Fprintf(&pdf, "%010d 00000 n \n", offset)
	}
	fmt.Fprintf(&pdf, "trailer\n<< /Size 4 /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", xref)
	return pdf.Bytes()
}
func TestRenderer_RealPoppler(t *testing.T) {
	if _, err := exec.LookPath("pdftoppm"); err != nil {
		t.Skip("pdftoppm absent; install poppler to run the local rendering check")
	}
	result, err := (PopplerRenderer{}).Render(context.Background(), smallPDF())
	if err != nil || len(result.Pages) != 1 {
		t.Fatalf("pages=%d error=%v", len(result.Pages), err)
	}
	config, err := png.DecodeConfig(bytes.NewReader(result.Pages[0]))
	if err != nil || config.Width != 150 || config.Height != 150 || config.ColorModel != color.GrayModel {
		t.Fatalf("config=%+v error=%v", config, err)
	}
}

func TestRenderer_MissingBinary(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	result, err := (PopplerRenderer{}).Render(context.Background(), smallPDF())
	if err == nil || err.Error() != "render_failed" || result.Stats.ExitCode != -1 {
		t.Fatalf("stats=%+v error=%v", result.Stats, err)
	}
}
func TestRenderer_CanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := (PopplerRenderer{}).Render(ctx, smallPDF())
	if err == nil || err.Error() != "render_failed" {
		t.Fatal("canceled render accepted")
	}
}
func TestCheck_RenderFailureRecordsSafeMetadata(t *testing.T) {
	r, s, c, d := fixture(t)
	r.Renderer = &fakeRenderer{err: fmt.Errorf("private-image-and-text")}
	if err := run(t, r, d, options()); err != nil {
		t.Fatal(err)
	}
	a := s.attempts[0]
	if c.DirectCalls != 0 || c.CountCalls != 0 || a.Reason != "render_failed" || a.Usage.Rendering == nil || a.Usage.Rendering.StderrBytes != 17 {
		t.Fatal("missing renderer failure metadata")
	}
}

func TestRenderer_PageOrderAndCleanup(t *testing.T) {
	var dir string
	renderer := PopplerRenderer{run: func(_ context.Context, work string) (RenderStats, error) {
		dir = work
		for page := 12; page >= 1; page-- {
			img := image.NewGray(image.Rect(0, 0, 1, 1))
			img.SetGray(0, 0, color.Gray{Y: uint8(page)})
			var data bytes.Buffer
			if err := png.Encode(&data, img); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(work, fmt.Sprintf("page-%d.png", page)), data.Bytes(), 0600); err != nil {
				t.Fatal(err)
			}
		}
		return RenderStats{}, nil
	}}
	result, err := renderer.Render(context.Background(), smallPDF())
	if err != nil || len(result.Pages) != 12 {
		t.Fatalf("pages=%d error=%v", len(result.Pages), err)
	}
	for i, data := range result.Pages {
		img, decodeErr := png.Decode(bytes.NewReader(data))
		if decodeErr != nil {
			t.Fatal(decodeErr)
		}
		if img.At(0, 0).(color.Gray).Y != uint8(i+1) {
			t.Fatal("pages rendered in wrong order")
		}
	}
	if _, err = os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("successful render directory retained")
	}
}
