package fetch

import (
	"context"
	"net/http"
	"testing"
	"testing/synctest"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/chinhphu"
	"danny.vn/trachuan/apps/corpus/internal/stage"
)

func unexpected206Fixture(t *testing.T, cap time.Duration) (*Stage, stage.Env, *int) {
	t.Helper()
	calls := 0
	const base = "https://test.invalid"
	c := chinhphu.New(chinhphu.WithBaseURLs(base, base), chinhphu.WithLimits(chinhphu.Limits{ItemTimeout: cap}), chinhphu.WithBackoffs(0, 0, 0), chinhphu.WithTransport(sourceTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 206, Header: http.Header{}, Body: &stalledStatusBody{ctx: r.Context(), closed: make(chan struct{})}}, nil
	})))
	return New(nil).WithChinhphu(c), stage.Env{Artifacts: artifact.Store{Root: t.TempDir()}}, &calls
}
func TestFetch_Unexpected206NearItemCapIsGap(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s, env, calls := unexpected206Fixture(t, time.Second)
		out, err := s.Process(context.Background(), env, stage.Run{Country: "vn"}, stage.Item{Key: "vn:chinhphu:218029"})
		if err != nil || *calls != 1 || out.State != "gap" || out.Artifact != nil || len(out.Gaps) != 1 || out.Gaps[0].Kind != "source_unavailable" || out.Gaps[0].HTTPStatus == nil || *out.Gaps[0].HTTPStatus != 206 {
			t.Fatalf("calls=%d output=%+v error=%v", *calls, out, err)
		}
	})
}
