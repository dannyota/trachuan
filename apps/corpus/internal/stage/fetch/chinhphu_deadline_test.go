package fetch

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/chinhphu"
	"danny.vn/trachuan/apps/corpus/internal/stage"
)

type pdfDeadlineTransport struct {
	pageRemaining time.Duration
	pdfRemaining  []time.Duration
	attempts      map[string]int
	cancel        context.CancelFunc
}

func (tr *pdfDeadlineTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	deadline, ok := req.Context().Deadline()
	if !ok {
		return nil, errors.New("request has no deadline")
	}
	remaining := time.Until(deadline)
	body := []byte(`<div id="ctrl_190596_91_Content"><table><tr><td>Tài liệu đính kèm</td><td><a href="https://test.invalid/a/first.pdf">first</a><a href="https://test.invalid/b/second.pdf">second</a></td></tr></table></div>`)
	media := "text/html"
	if strings.HasSuffix(req.URL.Path, ".pdf") {
		tr.pdfRemaining = append(tr.pdfRemaining, remaining)
		tr.attempts[req.URL.Path]++
		if tr.cancel != nil {
			tr.cancel()
			return nil, req.Context().Err()
		}
		if tr.attempts[req.URL.Path] == 1 {
			return nil, context.DeadlineExceeded
		}
		body = []byte("%PDF-test")
		media = "application/pdf"
	} else {
		tr.pageRemaining = remaining
	}
	return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{media}}, Body: io.NopCloser(bytes.NewReader(body)), Request: req}, nil
}
func deadlineStage(tr *pdfDeadlineTransport) *Stage {
	const base = "https://test.invalid"
	client := chinhphu.New(chinhphu.WithBaseURLs(base, base), chinhphu.WithTransport(tr), chinhphu.WithLimits(chinhphu.Limits{RequestsPerSecond: 100000, Burst: 10, ItemTimeout: time.Second, FileTimeout: time.Minute}), chinhphu.WithBackoffs(0))
	return New(nil).WithChinhphu(client)
}
func TestFetch_ChinhphuPDFDeadlinesOutlivePageCap(t *testing.T) {
	tr := &pdfDeadlineTransport{attempts: map[string]int{}}
	out, err := deadlineStage(tr).Process(context.Background(), stage.Env{Artifacts: artifact.Store{Root: t.TempDir()}}, stage.Run{Country: "vn"}, stage.Item{Key: "vn:chinhphu:218029"})
	if err != nil || out.Artifact == nil || out.State != "done" {
		t.Fatalf("output=%+v error=%v", out, err)
	}
	if tr.pageRemaining <= 0 || tr.pageRemaining > time.Second {
		t.Fatalf("page deadline=%s", tr.pageRemaining)
	}
	if len(tr.pdfRemaining) != 4 {
		t.Fatalf("PDF attempts=%d", len(tr.pdfRemaining))
	}
	for _, remaining := range tr.pdfRemaining {
		if remaining < 50*time.Second || remaining > time.Minute {
			t.Fatalf("PDF deadline=%s, clipped by page cap", remaining)
		}
	}
	for file, attempts := range tr.attempts {
		if attempts != 2 {
			t.Fatalf("%s attempts=%d", file, attempts)
		}
	}
}
func TestFetch_ChinhphuPDFKeepsActivityCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	tr := &pdfDeadlineTransport{attempts: map[string]int{}, cancel: cancel}
	out, err := deadlineStage(tr).Process(ctx, stage.Env{Artifacts: artifact.Store{Root: t.TempDir()}}, stage.Run{Country: "vn"}, stage.Item{Key: "vn:chinhphu:218029"})
	if !errors.Is(err, context.Canceled) || out.Artifact != nil {
		t.Fatalf("output=%+v error=%v", out, err)
	}
}
