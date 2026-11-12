package ocrvision

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

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
	paths, cleanup, err := (&memoryRenderer{pages: [][]byte{{1}, {2}}}).RenderJPEG(context.Background(), nil, 2)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	texts, outcome := r.pages(context.Background(), paths, newBudget(2))
	if outcome == "" || len(strings.Join(texts, "\n\n")) > 7 {
		t.Fatalf("retained=%d success=%v", len(strings.Join(texts, "\n\n")), outcome)
	}
	if maxTextBytes != 50_000_000 {
		t.Fatal("store text bound changed")
	}
}
