package fetch

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/chinhphu"
	"danny.vn/trachuan/apps/corpus/internal/stage"
)

type interruptedResponse struct{}

func (interruptedResponse) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }
func broken429Fixture(t *testing.T, pdf bool) (*Stage, stage.Env) {
	t.Helper()
	const base = "https://test.invalid"
	tr := sourceTransport(func(r *http.Request) (*http.Response, error) {
		if pdf && !strings.HasSuffix(r.URL.Path, ".pdf") {
			body := `<div id="ctrl_190596_91_Content"><table><tr><td>Tài liệu đính kèm</td><td><a href="https://test.invalid/a.pdf">a</a></td></tr></table></div>`
			return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"text/html"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
		}
		return &http.Response{StatusCode: 429, Header: http.Header{}, Body: io.NopCloser(interruptedResponse{})}, nil
	})
	c := chinhphu.New(chinhphu.WithBaseURLs(base, base), chinhphu.WithTransport(tr), chinhphu.WithLimits(chinhphu.Limits{RequestsPerSecond: 100000, Burst: 10}), chinhphu.WithBackoffs(0, 0, 0))
	return New(nil).WithChinhphu(c), stage.Env{Artifacts: artifact.Store{Root: t.TempDir()}}
}
func TestFetch_Broken429BodiesKeepItemsRetryable(t *testing.T) {
	for _, pdf := range []bool{false, true} {
		name := "page"
		if pdf {
			name = "PDF"
		}
		t.Run(name, func(t *testing.T) {
			s, env := broken429Fixture(t, pdf)
			out, err := s.Process(context.Background(), env, stage.Run{Country: "vn"}, stage.Item{Key: "vn:chinhphu:218029"})
			var retry *stage.RetryableError
			if !errors.As(err, &retry) || retry.Cause.Cause != stage.CauseHTTP429 || out.Artifact != nil {
				t.Fatalf("output=%+v error=%v", out, err)
			}
		})
	}
}
