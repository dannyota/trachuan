package fetch

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/chinhphu"
	"danny.vn/trachuan/apps/corpus/internal/sourcegate"
	"danny.vn/trachuan/apps/corpus/internal/stage"
)

type stalledStatusBody struct {
	ctx    context.Context
	closed chan struct{}
	once   sync.Once
}

func (b *stalledStatusBody) Read([]byte) (int, error) {
	select {
	case <-b.ctx.Done():
		return 0, b.ctx.Err()
	case <-b.closed:
		return 0, io.ErrClosedPipe
	}
}
func (b *stalledStatusBody) Close() error { b.once.Do(func() { close(b.closed) }); return nil }

func stalled429Fixture(t *testing.T, pdf bool, timeout time.Duration) (*Stage, stage.Env, *int, *[]time.Duration) {
	t.Helper()
	calls := 0
	waits := []time.Duration{}
	const base = "https://test.invalid"
	tr := sourceTransport(func(r *http.Request) (*http.Response, error) {
		if pdf && r.URL.Path != "/a.pdf" {
			return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"text/html"}}, Body: io.NopCloser(strings.NewReader(`<div id="ctrl_190596_91_Content"><table><tr><td>Tài liệu đính kèm</td><td><a href="https://test.invalid/a.pdf">a</a></td></tr></table></div>`))}, nil
		}
		calls++
		return &http.Response{StatusCode: 429, Header: http.Header{"Retry-After": []string{"3"}}, Body: &stalledStatusBody{ctx: r.Context(), closed: make(chan struct{})}}, nil
	})
	client := chinhphu.New(chinhphu.WithBaseURLs(base, base), chinhphu.WithTransport(tr), chinhphu.WithLimits(chinhphu.Limits{RequestsPerSecond: 100000, Burst: 10, PageTimeout: timeout, FileTimeout: timeout, ItemTimeout: 8 * time.Second}), chinhphu.WithBackoffs(0, 0, 0), chinhphu.WithClock(time.Now, func(_ context.Context, d time.Duration) error { waits = append(waits, d); return nil }))
	return New(nil).WithChinhphu(client), stage.Env{Artifacts: artifact.Store{Root: t.TempDir()}}, &calls, &waits
}
func TestFetch_Stalled429AtPageCapKeepsItemRetryable(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s, env, calls, waits := stalled429Fixture(t, false, 2*time.Second)
		out, err := s.Process(context.Background(), env, stage.Run{Country: "vn"}, stage.Item{Key: "vn:chinhphu:218029"})
		var retry *stage.RetryableError
		if *calls != 4 || len(*waits) != 3 || !errors.As(err, &retry) || retry.Cause.Cause != stage.CauseHTTP429 || out.Artifact != nil {
			t.Fatalf("calls=%d waits=%v output=%+v error=%v", *calls, *waits, out, err)
		}
		for _, wait := range *waits {
			if wait != 3*time.Second {
				t.Fatalf("Retry-After wait=%s", wait)
			}
		}
	})
}

func TestPDFBudget_FixedLoadOutcomeSurvivesAggregateDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s, _, _, _ := stalled429Fixture(t, true, 30*time.Second)
		activity := sourcegate.WithActivity(context.Background())
		ctx, cancel := context.WithTimeout(activity, pdfDownloadLimit)
		defer cancel()
		budget := newPDFBudget(time.Now)
		time.Sleep(pdfDownloadLimit - time.Second)
		dir := t.TempDir()
		if err := os.Mkdir(filepath.Join(dir, "files"), 0700); err != nil {
			t.Fatal(err)
		}
		_, err := budget.download(ctx, s, dir, "https://test.invalid/a.pdf")
		var failure *chinhphu.Failure
		if !errors.As(err, &failure) || failure.Cause != "http_429" {
			t.Fatalf("fixed load outcome=%v", err)
		}
	})
}
