package expand

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/vbpl"
	"golang.org/x/time/rate"
)

func TestExpand_MixedGatewayRetryFailures(t *testing.T) {
	for _, tc := range []struct {
		name       string
		statuses   []int
		wantCause  stage.Cause
		wantStatus int32
		retry      bool
	}{
		{"5xx then timeouts", []int{500}, stage.CauseTimeout, 0, false},
		{"last 5xx then timeouts", []int{500, 504}, stage.CauseHTTP5xx, 504, false},
		{"429 then timeouts", []int{429}, stage.CauseTimeout, 0, false},
		{"429 then 5xx and timeouts", []int{429, 500}, stage.CauseTimeout, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if !strings.HasSuffix(r.URL.Path, "/diagram") {
					_, _ = w.Write([]byte(`{"success":true,"data":{"id":"1"}}`))
					return
				}
				i := int(calls.Add(1)) - 1
				if i < len(tc.statuses) {
					w.WriteHeader(tc.statuses[i])
					return
				}
				<-r.Context().Done()
			}))
			defer server.Close()
			client := vbpl.NewWithBase(server.URL, "test", vbpl.WithLimits(float64(rate.Inf), 8), vbpl.WithBackoffs(time.Millisecond, time.Millisecond, time.Millisecond), vbpl.WithTestTimeouts(5*time.Millisecond, 60*time.Millisecond))
			out, err := New(client).Process(context.Background(), stage.Env{}, stage.Run{Country: "vn"}, stage.Item{Key: "vn:vbpl:1"})
			wantCalls := int32(2)
			if tc.statuses[0] == 429 {
				wantCalls = 3
			}
			if calls.Load() != wantCalls {
				t.Fatalf("calls = %d", calls.Load())
			}

			if err != nil || out.State != "gap" || len(out.Gaps) != 1 || out.Gaps[0].Kind != "source_unavailable" || out.Gaps[0].Cause != tc.wantCause || out.Gaps[0].Endpoint != "doc/diagram" || (tc.wantStatus == 0 && out.Gaps[0].HTTPStatus != nil) || (tc.wantStatus != 0 && (out.Gaps[0].HTTPStatus == nil || *out.Gaps[0].HTTPStatus != tc.wantStatus)) {
				t.Fatalf("out=%+v err=%v", out, err)
			}
		})
	}
}
