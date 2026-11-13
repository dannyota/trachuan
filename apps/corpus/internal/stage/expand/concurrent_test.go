package expand

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"testing/synctest"

	"danny.vn/trachuan/apps/corpus/internal/sourcegate"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/vbpl"
	"golang.org/x/time/rate"
)

type concurrentTransport func(*http.Request) (*http.Response, error)

func (f concurrentTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestExpand_DiagramFailureWaitsForDetailMismatch(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		detailStarted := make(chan struct{})
		diagramStarted := make(chan struct{})
		releaseDetail := make(chan struct{})
		transport := concurrentTransport(func(r *http.Request) (*http.Response, error) {
			status, body := http.StatusOK, `{"success":true,"data":{"id":"2"}}`
			if strings.HasSuffix(r.URL.Path, "/diagram") {
				close(diagramStarted)
				status, body = http.StatusBadRequest, ""
			} else {
				close(detailStarted)
				<-releaseDetail
			}
			return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header), Request: r}, nil
		})
		client := vbpl.NewWithBase("https://example.test", "test", vbpl.WithTransport(transport), vbpl.WithLimits(float64(rate.Inf), 8))
		done := make(chan stage.Output, 1)
		go func() {
			out, _ := New(client).Process(context.Background(), stage.Env{}, stage.Run{Country: "vn"}, stage.Item{Key: "vn:vbpl:1"})
			done <- out
		}()
		synctest.Wait()
		select {
		case <-detailStarted:
		default:
			t.Fatal("detail did not start")
		}
		select {
		case <-diagramStarted:
		default:
			close(releaseDetail)
			<-done
			t.Fatal("diagram did not start while detail was in flight")
		}
		close(releaseDetail)
		out := <-done
		if out.State != "gap" || len(out.Gaps) != 1 || out.Gaps[0].Kind != "detail_id_mismatch" {
			t.Fatalf("output = %#v", out)
		}
	})
}

func TestExpand_PauseCancelsSibling(t *testing.T) {
	for _, paused := range []string{"detail", "diagram"} {
		t.Run(paused, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				started := make(chan struct{})
				cancelled := make(chan struct{})
				transport := concurrentTransport(func(r *http.Request) (*http.Response, error) {
					name := "detail"
					if strings.HasSuffix(r.URL.Path, "/diagram") {
						name = "diagram"
					}
					if name != paused {
						close(started)
						<-r.Context().Done()
						close(cancelled)
						return nil, r.Context().Err()
					}
					<-started
					return &http.Response{StatusCode: http.StatusForbidden, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header), Request: r}, nil
				})
				client := vbpl.NewWithBase("https://example.test", "test", vbpl.WithTransport(transport), vbpl.WithLimits(float64(rate.Inf), 8), vbpl.WithGate(sourcegate.NewMemory()))
				ctx := sourcegate.WithRunID(sourcegate.WithActivity(context.Background()), 1)
				ctx, telemetry := sourcegate.WithTelemetry(ctx)
				_, err := New(client).Process(ctx, stage.Env{}, stage.Run{Country: "vn"}, stage.Item{Key: "vn:vbpl:1"})
				if !errors.Is(err, sourcegate.ErrSourcePaused) {
					t.Fatalf("error = %v", err)
				}
				select {
				case <-cancelled:
				default:
					t.Fatal("sibling was not cancelled")
				}
				seen := map[string]string{}
				for _, row := range telemetry.Records() {
					seen[row.Endpoint] = row.Outcome
				}
				other := "detail"
				if paused == "detail" {
					other = "diagram"
				}
				if seen[paused] != "paused" || seen[other] != "cancelled" {
					t.Fatalf("telemetry = %v", seen)
				}
			})
		})
	}
}

func TestExpand_DetailFailureCancelsDiagram(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		diagramStarted := make(chan struct{})
		cancelled := make(chan struct{})
		transport := concurrentTransport(func(r *http.Request) (*http.Response, error) {
			if strings.HasSuffix(r.URL.Path, "/diagram") {
				close(diagramStarted)
				<-r.Context().Done()
				close(cancelled)
				return nil, r.Context().Err()
			}
			<-diagramStarted
			return &http.Response{StatusCode: http.StatusNotFound, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header), Request: r}, nil
		})
		client := vbpl.NewWithBase("https://example.test", "test", vbpl.WithTransport(transport), vbpl.WithLimits(float64(rate.Inf), 8))
		out, err := New(client).Process(context.Background(), stage.Env{}, stage.Run{Country: "vn"}, stage.Item{Key: "vn:vbpl:1"})
		if err != nil || out.State != "gap" || len(out.Gaps) != 1 || out.Gaps[0].Kind != "not_found" {
			t.Fatalf("out=%#v err=%v", out, err)
		}
		select {
		case <-cancelled:
		default:
			t.Fatal("diagram was not cancelled")
		}
	})
}
