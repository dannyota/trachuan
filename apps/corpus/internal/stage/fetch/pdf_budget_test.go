package fetch

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/chinhphu"
	"danny.vn/trachuan/apps/corpus/internal/stage"
)

type sourceTransport func(*http.Request) (*http.Response, error)

func (f sourceTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestFetch_ChinhphuPDFsShareTenMinutes(t *testing.T) {
	now := time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC)
	calls := 0
	const base = "https://test.invalid"
	page := `<div id="ctrl_190596_91_Content"><table><tr><td>Tài liệu đính kèm</td><td>`
	for i := 0; i < 5; i++ {
		page += fmt.Sprintf(`<a href="%s/%d.pdf">%d</a>`, base, i, i)
	}
	page += `</td></tr></table></div>`
	tr := sourceTransport(func(r *http.Request) (*http.Response, error) {
		body, media := page, "text/html"
		if strings.HasSuffix(r.URL.Path, ".pdf") {
			calls++
			deadline, ok := r.Context().Deadline()
			want := 230 * time.Second
			if calls == 3 {
				want = 200 * time.Second
			}
			if !ok || time.Until(deadline) > want+time.Second {
				t.Errorf("attempt deadline not bounded: %s", time.Until(deadline))
			}
			now = now.Add(200 * time.Second)
			body, media = "%PDF-test", "application/pdf"
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{media}}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	c := chinhphu.New(chinhphu.WithBaseURLs(base, base), chinhphu.WithTransport(tr), chinhphu.WithLimits(chinhphu.Limits{RequestsPerSecond: 100000, Burst: 10}), chinhphu.WithClock(func() time.Time { return now }, func(context.Context, time.Duration) error { return nil }))
	s := New(nil).WithChinhphu(c)
	env := stage.Env{Artifacts: artifact.Store{Root: t.TempDir()}, Now: func() time.Time { return now }}
	out, err := s.Process(context.Background(), env, stage.Run{Country: "vn"}, stage.Item{Key: "vn:chinhphu:218029"})
	if err != nil || out.State != "done" || out.Artifact == nil || calls != 3 || parseReadMessage(out.Message).counts["failure_deadline"] != 2 {
		t.Fatalf("output=%+v calls=%d error=%v", out, calls, err)
	}
	res, err := s.Resolve(context.Background(), env, stage.Run{}, stage.Item{Key: "vn:chinhphu:218029"}, *out.Artifact)
	if err != nil || len(res.Gaps) != 2 {
		t.Fatalf("gaps=%+v error=%v", res, err)
	}
	for _, gap := range res.Gaps {
		if gap.Kind != "missing_attachment" || strings.Contains(gap.Detail, "deadline") {
			t.Fatalf("gap=%+v", gap)
		}
	}
}

func TestPDFBudget_ActiveDeadlineBecomesMissingCause(t *testing.T) {
	now := time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC)
	budget := newPDFBudget(func() time.Time { return now })
	now = now.Add(590 * time.Second)
	calls := 0
	tr := sourceTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		deadline, ok := r.Context().Deadline()
		if !ok || time.Until(deadline) > 10*time.Second {
			t.Error("aggregate deadline did not clip attempt")
		}
		now = now.Add(10 * time.Second)
		return nil, context.DeadlineExceeded
	})
	c := chinhphu.New(chinhphu.WithBaseURLs("https://test.invalid", "https://test.invalid"), chinhphu.WithTransport(tr), chinhphu.WithBackoffs())
	s := New(nil).WithChinhphu(c)
	dir := t.TempDir()
	if err := os.Mkdir(dir+"/files", 0700); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		_, err := budget.download(context.Background(), s, dir, "https://test.invalid/a.pdf")
		if !errors.Is(err, errPDFDeadline) {
			t.Fatalf("error=%v", err)
		}
	}
	if calls != 1 {
		t.Fatalf("requests after aggregate deadline=%d", calls)
	}
}
