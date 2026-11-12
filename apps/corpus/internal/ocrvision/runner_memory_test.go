package ocrvision

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/ocrcheck"
)

type allowanceRenderer struct{ allowance int }

func (*allowanceRenderer) Preflight(context.Context) error { return nil }
func (r *allowanceRenderer) RenderLimited(_ context.Context, _ []byte, limits ocrcheck.RenderLimits) (ocrcheck.Rendered, error) {
	r.allowance = limits.TotalBytes
	return ocrcheck.Rendered{Pages: [][]byte{{1}}}, nil
}

func TestRunner_RenderAllowanceIsBounded(t *testing.T) {
	c := fakeClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"responses":[{"fullTextAnnotation":{"text":"ok"}}]}`)
	})
	renderer := &allowanceRenderer{}
	r := Runner{Client: c, Renderer: renderer, Store: &memoryStore{}, CountPages: func(context.Context, []byte) (int, error) { return 1, nil }}
	if _, err := r.Run(context.Background(), []Document{document(t)}, Options{MaxPages: 1}, io.Discard); err != nil {
		t.Fatal(err)
	}
	if renderer.allowance <= 0 || renderer.allowance > 512_000_000 {
		t.Fatalf("unbounded rendered allowance: %d", renderer.allowance)
	}
}

func TestRunner_TextLimitIncludesSeparatorsAndStopsRetention(t *testing.T) {
	c := fakeClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"responses":[{"fullTextAnnotation":{"text":"abc"}}]}`)
	})
	source := &memoryStore{}
	r := Runner{Client: c, Renderer: &memoryRenderer{pages: [][]byte{{1}, {2}}}, Store: source, CountPages: func(context.Context, []byte) (int, error) { return 2, nil }, textLimit: 7}
	summary, err := r.Run(context.Background(), []Document{document(t)}, Options{MaxPages: 2}, io.Discard)
	if err == nil || summary.Failed != 1 || source.calls != 0 {
		t.Fatalf("summary=%+v err=%v writes=%d", summary, err, source.calls)
	}
	texts, ok := r.pages(context.Background(), [][]byte{{1}, {2}}, newBudget(2))
	if ok || len(strings.Join(texts, "\n\n")) > 7 {
		t.Fatalf("retained=%d success=%v", len(strings.Join(texts, "\n\n")), ok)
	}
	if maxTextBytes != 50_000_000 {
		t.Fatal("store text bound changed")
	}
}
