package ocrvision

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type interruptedRenderer struct {
	mode   string
	cancel context.CancelFunc
}

func (interruptedRenderer) Preflight(context.Context) error { return nil }
func (r interruptedRenderer) RenderJPEG(ctx context.Context, _ []byte, _ int) ([]string, func(), error) {
	if r.mode == "deadline" {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 10*time.Millisecond)
		defer cancel()
	} else {
		r.cancel()
	}
	<-ctx.Done()
	return nil, nil, errors.New("render_failed")
}

func TestRunner_InterruptedRenderSendsNothing(t *testing.T) {
	for _, mode := range []string{"cancel", "deadline"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var requests atomic.Int64
			client := fakeClient(t, func(http.ResponseWriter, *http.Request) { requests.Add(1) })
			store := &memoryStore{}
			runner := Runner{Client: client, Store: store, Renderer: interruptedRenderer{mode: mode, cancel: cancel}, CountPages: func(context.Context, []byte) (int, error) { return 1, nil }}
			var out strings.Builder
			summary, err := runner.Run(ctx, []Document{document(t)}, Options{MaxPages: 1}, &out)
			if err == nil || summary.Failed != 1 || requests.Load() != 0 || store.calls != 0 || !strings.Contains(out.String(), "outcome=render_failed") {
				t.Fatalf("summary=%+v requests=%d stored=%d err=%v output=%q", summary, requests.Load(), store.calls, err, out.String())
			}
		})
	}
}
