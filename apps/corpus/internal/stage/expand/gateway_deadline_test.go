package expand

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/vbpl"
	"golang.org/x/time/rate"
)

func TestExpand_DelayedGatewayTimeoutIs5xxGap(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		transport := mixedGatewayTransport(func(r *http.Request) (*http.Response, error) {
			select {
			case <-time.After(32 * time.Millisecond):
				return &http.Response{StatusCode: http.StatusGatewayTimeout, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("")), Request: r}, nil
			case <-r.Context().Done():
				return nil, r.Context().Err()
			}
		})
		client := vbpl.NewWithBase("https://example.test", "test", vbpl.WithTransport(transport), vbpl.WithLimits(float64(rate.Inf), 8), vbpl.WithBackoffs(), vbpl.WithTestTimeouts(40*time.Millisecond, 200*time.Millisecond))
		out, err := New(client).Process(context.Background(), stage.Env{}, stage.Run{Country: "vn"}, stage.Item{Key: "vn:vbpl:1"})
		if err != nil || out.State != "gap" || len(out.Gaps) != 1 || out.Gaps[0].Kind != "source_unavailable" || out.Gaps[0].Cause != stage.CauseHTTP5xx || out.Gaps[0].Origin != stage.OriginGateway {
			t.Fatalf("output = %+v, error = %v", out, err)
		}
	})
}
