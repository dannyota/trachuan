package expand

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/vbpl"
	"golang.org/x/time/rate"
)

func TestExpand_DelayedGatewayTimeoutIs5xxGap(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(32 * time.Millisecond):
			w.WriteHeader(http.StatusGatewayTimeout)
		case <-r.Context().Done():
		}
	}))
	defer server.Close()
	client := vbpl.NewWithBase(server.URL, "test", vbpl.WithLimits(float64(rate.Inf), 8), vbpl.WithBackoffs(), vbpl.WithTestTimeouts(40*time.Millisecond, 200*time.Millisecond))
	out, err := New(client).Process(context.Background(), stage.Env{}, stage.Run{Country: "vn"}, stage.Item{Key: "vn:vbpl:1"})
	if err != nil || out.State != "gap" || len(out.Gaps) != 1 || out.Gaps[0].Kind != "source_unavailable" || out.Gaps[0].Cause != stage.CauseHTTP5xx || out.Gaps[0].Origin != stage.OriginGateway {
		t.Fatalf("output = %+v, error = %v", out, err)
	}
}
