package expand

import (
	"context"
	"errors"
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
		{"5xx then timeouts", []int{500}, stage.CauseHTTP5xx, 500, false},
		{"last 5xx then timeouts", []int{500, 504}, stage.CauseHTTP5xx, 504, false},
		{"429 then timeouts", []int{429}, stage.CauseHTTP429, 429, true},
		{"429 then 5xx and timeouts", []int{429, 500}, stage.CauseHTTP429, 429, true},
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
			if calls.Load() != 4 {
				t.Fatalf("calls = %d", calls.Load())
			}
			if tc.retry {
				var retry *stage.RetryableError
				if !errors.As(err, &retry) || retry.GapKind != "source_unavailable" || retry.Cause.Cause != tc.wantCause || retry.Cause.Endpoint != "doc/diagram" || retry.Cause.HTTPStatus == nil || *retry.Cause.HTTPStatus != tc.wantStatus {
					t.Fatalf("out=%+v err=%v", out, err)
				}
				return
			}
			if err != nil || out.State != "gap" || len(out.Gaps) != 1 || out.Gaps[0].Kind != "source_unavailable" || out.Gaps[0].Cause != tc.wantCause || out.Gaps[0].Origin != stage.OriginGateway || out.Gaps[0].Endpoint != "doc/diagram" || out.Gaps[0].HTTPStatus == nil || *out.Gaps[0].HTTPStatus != tc.wantStatus {
				t.Fatalf("out=%+v err=%v", out, err)
			}
		})
	}
}
