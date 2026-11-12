package ocrcheck

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestVersion_Settings(t *testing.T) {
	for _, tc := range []struct {
		version, dpi int
		media        string
		direct       bool
	}{
		{0, 150, "image/png", false}, {2, 150, "image/png", false},
		{3, 200, "image/jpeg", true}, {4, 150, "image/jpeg", true},
	} {
		t.Run(fmt.Sprint(tc.version), func(t *testing.T) {
			o := options()
			o.CheckVersion = tc.version
			v, err := o.version()
			if err != nil || v.dpi != tc.dpi || v.mediaType != tc.media || v.directOnly != tc.direct {
				t.Fatalf("settings=%+v err=%v", v, err)
			}
			n := tc.version
			if n == 0 {
				n = 2
			}
			if want := fmt.Sprintf("%s/%s/ocr-check-%d", o.Model, o.Effort, n); o.CheckerID() != want {
				t.Fatal(o.CheckerID())
			}
			o.Batch = true
			if err = o.Validate(); (err != nil) != tc.direct {
				t.Fatalf("batch err=%v", err)
			}
		})
	}
	o := options()
	o.CheckVersion = 1
	if o.Validate() == nil {
		t.Fatal("accepted unsupported version")
	}
}

type jpegFixtureRenderer struct {
	paths   []string
	err     error
	dpi     int
	cleaned bool
}

func (f *jpegFixtureRenderer) Preflight(context.Context) error { return nil }
func (f *jpegFixtureRenderer) RenderJPEGDPI(_ context.Context, _ []byte, _, dpi int) ([]string, func(), error) {
	f.dpi = dpi
	return f.paths, func() { f.cleaned = true }, f.err
}
func jpegFixture(t *testing.T) (*Runner, *memoryRepository, *FakeClient, []Document, *jpegFixtureRenderer) {
	t.Helper()
	r, repo, c, docs := fixture(t)
	f := &jpegFixtureRenderer{}
	dir := t.TempDir()
	for i := 0; i < 10; i++ {
		path := filepath.Join(dir, fmt.Sprintf("%d.jpg", i))
		// The renderer owns JPEG validation; these bytes identify page order.
		if err := os.WriteFile(path, []byte(fmt.Sprintf("jpeg-page-%d", i)), 0600); err != nil {
			t.Fatal(err)
		}
		f.paths = append(f.paths, path)
	}
	r.JPEGRenderer = f
	return r, repo, c, docs, f
}
func jpegOptions() Options { o := options(); o.CheckVersion = 3; return o }

func TestVersion_JPEGRequest(t *testing.T) {
	for _, version := range []int{3, 4} {
		t.Run(fmt.Sprint(version), func(t *testing.T) {
			r, _, c, docs, f := jpegFixture(t)
			o := jpegOptions()
			o.CheckVersion = version
			o.DryRun = true
			if err := run(t, r, docs, o); err != nil {
				t.Fatal(err)
			}
			if len(c.Counted) != 1 || !f.cleaned {
				t.Fatalf("counts=%d cleanup=%v", len(c.Counted), f.cleaned)
			}
			wantDPI := 200
			if version == 4 {
				wantDPI = 150
			}
			if f.dpi != wantDPI {
				t.Fatal(f.dpi)
			}
			got := messageParams(c.Counted[0])
			png := c.Counted[0]
			png.MediaType = ""
			old := messageParams(png)
			g, _ := json.Marshal(got.OutputConfig)
			w, _ := json.Marshal(old.OutputConfig)
			if string(g) != string(w) || !reflect.DeepEqual(got.Messages[0].OfUser.Content[20:], old.Messages[0].OfUser.Content[20:]) {
				t.Fatal("instruction or schema changed")
			}
			b, _ := json.Marshal(got)
			last := -1
			for i := 0; i < 10; i++ {
				needle := base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("jpeg-page-%d", i)))
				at := strings.Index(string(b), needle)
				if at <= last {
					t.Fatalf("page %d out of order", i)
				}
				last = at
			}
			if strings.Count(string(b), `"media_type":"image/jpeg"`) != 10 || strings.Contains(string(b), "image/png") {
				t.Fatal("wrong media type")
			}
		})
	}
}

func TestVersion_SizeOrder(t *testing.T) {
	for _, mode := range []string{"pages", "body", "assembled", "context", "boundary"} {
		t.Run(mode, func(t *testing.T) {
			r, repo, c, docs, f := jpegFixture(t)
			switch mode {
			case "pages":
				r.CountPages = func(context.Context, []byte) (int, error) { return 501, nil }
			case "body":
				docs[0].Text = []byte(strings.Repeat("x", maxRequestBytes))
			case "assembled":
				docs[0].Text = []byte(strings.Repeat("x", 6_666_000))
				page := []byte(strings.Repeat("j", 7_000_000))
				if err := os.WriteFile(f.paths[0], page, 0600); err != nil {
					t.Fatal(err)
				}
				for i := range f.paths {
					f.paths[i] = f.paths[0]
				}
			case "context":
				c.Tokens = 968001
			case "boundary":
				c.Tokens = 968000
			}
			o := jpegOptions()
			o.MaxUSD = 100
			if err := run(t, r, docs, o); err != nil {
				t.Fatal(err)
			}
			if mode == "boundary" {
				if c.DirectCalls != 1 {
					t.Fatal("context boundary refused")
				}
				return
			}
			wantCount := 0
			if mode == "context" {
				wantCount = 1
			}
			if c.CountCalls != wantCount || c.DirectCalls != 0 || len(repo.attempts) != 1 || repo.attempts[0].Reason != "over_check_limit" || repo.attempts[0].Usage.charge() != 0 {
				t.Fatalf("counts=%d calls=%d attempts=%+v", c.CountCalls, c.DirectCalls, repo.attempts)
			}
			if mode == "context" && !f.cleaned {
				t.Fatal("scratch leaked")
			}
			s, err := r.Run(context.Background(), docs, o)
			if err != nil || s.SkippedRetryLimit != 1 {
				t.Fatalf("retry=%+v err=%v", s, err)
			}
		})
	}
}

func TestVersion_RenderFailureCleanup(t *testing.T) {
	for _, tc := range []struct {
		err    error
		reason string
	}{{PageTooLarge, "over_check_limit"}, {RenderTooLarge, "over_check_limit"}, {renderFailed, "render_failed"}} {
		r, repo, c, docs, f := jpegFixture(t)
		f.err = tc.err
		if err := run(t, r, docs, jpegOptions()); err != nil {
			t.Fatal(err)
		}
		if !f.cleaned || c.CountCalls != 0 || c.DirectCalls != 0 || repo.attempts[0].Reason != tc.reason {
			t.Fatalf("cleanup=%v attempts=%+v", f.cleaned, repo.attempts)
		}
	}
}
