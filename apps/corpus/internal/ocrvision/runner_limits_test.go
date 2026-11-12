package ocrvision

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/ocrcheck"
)

func TestRunner_FourConcurrentPages(t *testing.T) {
	started := make(chan struct{}, 8)
	release := make(chan struct{})
	var active, maxActive atomic.Int64
	c := fakeClient(t, func(w http.ResponseWriter, r *http.Request) {
		n := active.Add(1)
		for old := maxActive.Load(); old < n; old = maxActive.Load() {
			if maxActive.CompareAndSwap(old, n) {
				break
			}
		}
		started <- struct{}{}
		select {
		case <-release:
		case <-r.Context().Done():
		}
		active.Add(-1)
		_, _ = io.WriteString(w, `{"responses":[{"fullTextAnnotation":{"text":"ok"}}]}`)
	})
	render := &memoryRenderer{pages: make([][]byte, 8)}
	runner := Runner{Client: c, Store: &memoryStore{}, Renderer: render, CountPages: func(context.Context, []byte) (int, error) { return 8, nil }}
	done := make(chan error, 1)
	go func() {
		_, err := runner.Run(context.Background(), []Document{document(t)}, Options{MaxPages: 8}, io.Discard)
		done <- err
	}()
	for range 4 {
		<-started
	}
	select {
	case <-started:
		t.Error("more than four pages in flight")
	default:
	}
	close(release)
	if err := <-done; err != nil || maxActive.Load() != 4 {
		t.Fatalf("max=%d err=%v", maxActive.Load(), err)
	}
}

func TestRunner_InvocationBudgetAndLimitDocs(t *testing.T) {
	var calls atomic.Int64
	c := fakeClient(t, func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		_, _ = io.WriteString(w, `{"responses":[{"fullTextAnnotation":{"text":"ok"}}]}`)
	})
	runner := Runner{Client: c, Store: &memoryStore{}, Renderer: &memoryRenderer{pages: [][]byte{{1}}}, CountPages: func(context.Context, []byte) (int, error) { return 1, nil }}
	docs := []Document{document(t), document(t), document(t)}
	var out strings.Builder
	summary, err := runner.Run(context.Background(), docs, Options{MaxPages: 2}, &out)
	if err != nil || summary.Stored != 2 || summary.Skipped != 1 || calls.Load() != 2 || !strings.HasSuffix(out.String(), "stored=2 present=0 failed=0 skipped_limit=1 requests=2\n") {
		t.Fatalf("summary=%+v err=%v output=%s", summary, err, out.String())
	}
	summary, err = runner.Run(context.Background(), docs, Options{MaxPages: 2, LimitDocs: 1}, io.Discard)
	if err != nil || summary.Documents != 1 {
		t.Fatalf("summary=%+v err=%v", summary, err)
	}
}

func TestRunner_AuthenticationBeforeRendering(t *testing.T) {
	render := &memoryRenderer{}
	runner := Runner{Client: NewClient(Config{}), Renderer: render}
	if _, err := runner.Run(context.Background(), []Document{document(t)}, Options{MaxPages: 1}, io.Discard); err == nil || render.calls != 0 {
		t.Fatal("rendered without credentials")
	}
}

func TestRunner_UnsafePDFs(t *testing.T) {
	for _, kind := range []string{"missing", "directory", "symlink", "oversize"} {
		t.Run(kind, func(t *testing.T) {
			d := document(t)
			switch kind {
			case "missing":
				d.Path += "missing"
			case "directory":
				d.Path = filepath.Dir(d.Path)
			case "symlink":
				path := d.Path + "link"
				if err := os.Symlink(d.Path, path); err != nil {
					t.Fatal(err)
				}
				d.Path = path
			case "oversize":
				if err := os.Truncate(d.Path, 50_000_001); err != nil {
					t.Fatal(err)
				}
			}
			if _, reason := readPDF(d); reason != "failed" {
				t.Fatalf("reason=%s", reason)
			}
		})
	}
}

func TestRunner_CountAndRenderFailures(t *testing.T) {
	for _, tc := range []struct {
		name                string
		count               int
		countErr, renderErr error
	}{
		{name: "count error", countErr: fmt.Errorf("untrusted parser error")},
		{name: "zero pages"},
		{name: "render error", count: 1, renderErr: fmt.Errorf("untrusted renderer error")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int64
			c := fakeClient(t, func(http.ResponseWriter, *http.Request) { calls.Add(1) })
			s := &memoryStore{}
			r := Runner{Client: c, Store: s, Renderer: &memoryRenderer{err: tc.renderErr}, CountPages: func(context.Context, []byte) (int, error) { return tc.count, tc.countErr }}
			var out strings.Builder
			if summary, err := r.Run(context.Background(), []Document{document(t)}, Options{MaxPages: 10}, &out); err == nil || summary.Failed != 1 || calls.Load() != 0 || s.calls != 0 || strings.Contains(out.String(), "untrusted") {
				t.Fatalf("summary=%+v err=%v", summary, err)
			}
		})
	}
}

func TestRunner_DryRunNeedsNoClientOrRenderer(t *testing.T) {
	r := Runner{CountPages: func(context.Context, []byte) (int, error) { return 2, nil }}
	var out strings.Builder
	d := document(t)
	if _, err := r.Run(context.Background(), []Document{d}, Options{MaxPages: 1, DryRun: true}, &out); err != nil || out.String() != fmt.Sprintf("pdf=%s pages=2\ndocuments=1 pages=2 failed=0\n", d.Hash) {
		t.Fatalf("output=%q err=%v", out.String(), err)
	}
}

func TestRunner_SameBytesAreRendered(t *testing.T) {
	// The renderer sees the bytes already read and hashed, even after replacement.
	d := document(t)
	c := fakeClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"responses":[{"fullTextAnnotation":{"text":"ok"}}]}`)
	})
	r := Runner{Client: c, Store: &memoryStore{}, Renderer: checkingRenderer{t: t}, CountPages: func(_ context.Context, data []byte) (int, error) {
		if err := os.WriteFile(d.Path, []byte("replacement"), 0600); err != nil {
			t.Fatal(err)
		}
		return 1, nil
	}}
	if _, err := r.Run(context.Background(), []Document{d}, Options{MaxPages: 1}, io.Discard); err != nil {
		t.Fatal(err)
	}
}

type checkingRenderer struct{ t *testing.T }

func (checkingRenderer) Preflight(context.Context) error { return nil }
func (r checkingRenderer) RenderLimited(_ context.Context, data []byte, _ ocrcheck.RenderLimits) (ocrcheck.Rendered, error) {
	if string(data) != "verified PDF" {
		r.t.Error("rendered replacement bytes")
	}
	return ocrcheck.Rendered{Pages: [][]byte{{1}}}, nil
}

func TestRunner_NoSecretsInOutputOrLogs(t *testing.T) {
	var out, logs strings.Builder
	previous := log.Writer()
	log.SetOutput(&logs)
	defer log.SetOutput(previous)
	c := fakeClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"responses":[{"error":{"code":3,"message":"private-token private-project"}}]}`)
	})
	source := &memoryStore{}
	runner := Runner{Client: c, Store: source, Renderer: &memoryRenderer{pages: [][]byte{{1}}}, CountPages: func(context.Context, []byte) (int, error) { return 1, nil }}
	_, err := runner.Run(context.Background(), []Document{document(t)}, Options{MaxPages: 1}, &out)
	if err == nil || source.calls != 0 || strings.Contains(out.String()+logs.String()+err.Error(), "private-") {
		t.Fatal("credentials disclosed or stored")
	}
}
