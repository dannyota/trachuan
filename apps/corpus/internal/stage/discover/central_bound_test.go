package discover

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"testing"
	"testing/synctest"

	"danny.vn/trachuan/apps/corpus/internal/lookup"
	"danny.vn/trachuan/apps/corpus/internal/sourcegate"
	"danny.vn/trachuan/apps/corpus/internal/vbpl"
	"golang.org/x/time/rate"
)

func listingPage(total any, first, count int) string {
	items := make([]map[string]string, count)
	for i := range items {
		items[i] = map[string]string{"id": fmt.Sprint(first + i)}
	}
	raw, _ := json.Marshal(map[string]any{"success": true, "data": map[string]any{"total": total, "items": items}})
	return string(raw)
}

func TestCentralPass_NoSpeculationBeyondReportedPages(t *testing.T) {
	for _, tc := range []struct {
		name               string
		total, pages, last int
	}{{"one", 1000, 1, 1000}, {"two", 1001, 2, 1}, {"full last", 2000, 2, 1000}} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			synctest.Test(t, func(t *testing.T) {
				var mu sync.Mutex
				var requests []int
				release := make(chan struct{})
				transport := centralTransport(func(r *http.Request) (*http.Response, error) {
					var q struct {
						Page int `json:"pageNumber"`
					}
					if err := json.NewDecoder(r.Body).Decode(&q); err != nil {
						return nil, err
					}
					mu.Lock()
					requests = append(requests, q.Page)
					mu.Unlock()
					if q.Page > tc.pages {
						return &http.Response{StatusCode: 403, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("")), Request: r}, nil
					}
					count := 1000
					if q.Page == tc.pages {
						count = tc.last
					}
					if q.Page == 2 {
						<-release
					}
					return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(listingPage(tc.total, (q.Page-1)*1000, count))), Request: r}, nil
				})
				client := vbpl.NewWithBase("https://example.test", "test", vbpl.WithTransport(transport), vbpl.WithLimits(float64(rate.Inf), 16))
				done := make(chan error, 1)
				go func() {
					_, _, err := New(client).centralPass(context.Background(), dir, 1, "test", json.RawMessage(`{}`))
					done <- err
				}()
				synctest.Wait()
				close(release)
				err := <-done
				mu.Lock()
				defer mu.Unlock()
				for _, page := range requests {
					if page > tc.pages {
						t.Errorf("requested page %d above %d", page, tc.pages)
					}
				}
				if err != nil {
					t.Fatalf("pass error=%v; requests=%v", err, requests)
				}
			})
		})
	}
}

func TestCentralPass_DuplicateRequestsExtraPageOnDemand(t *testing.T) {
	for _, pause := range []bool{false, true} {
		t.Run(fmt.Sprint(pause), func(t *testing.T) {
			dir, sequentialDir := t.TempDir(), t.TempDir()
			synctest.Test(t, func(t *testing.T) {
				var mu sync.Mutex
				seen := map[int]int{}
				release := make(chan struct{})
				transport := centralTransport(func(r *http.Request) (*http.Response, error) {
					var q struct {
						Page int `json:"pageNumber"`
					}
					if err := json.NewDecoder(r.Body).Decode(&q); err != nil {
						return nil, err
					}
					mu.Lock()
					seen[q.Page]++
					mu.Unlock()
					if q.Page == 2 {
						<-release
					}
					status, body := 200, listingPage(1002, 0, 0)
					switch q.Page {
					case 1:
						body = listingPage(1002, 0, 1000)
					case 2:
						body = `{"success":true,"data":{"total":1002,"items":[{"id":"0"},{"id":"1000"}]}}`
					case 3:
						if pause {
							status = 403
						}
					}
					return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
				})
				client := vbpl.NewWithBase("https://example.test", "test", vbpl.WithTransport(transport), vbpl.WithLimits(float64(rate.Inf), 16))
				type output struct {
					result lookup.Result
					refs   map[string]pageReference
					err    error
				}
				done := make(chan output, 1)
				go func() {
					result, refs, err := New(client).centralPass(context.Background(), dir, 1, "test", json.RawMessage(`{}`))
					done <- output{result, refs, err}
				}()
				synctest.Wait()
				mu.Lock()
				early := seen[3]
				mu.Unlock()
				close(release)
				actual := <-done
				if early != 0 {
					t.Errorf("page 3 was requested before page 2 completed")
				}
				expected, refs, err := New(client).centralPassSequential(context.Background(), sequentialDir, 1, "test", json.RawMessage(`{}`))
				if pause {
					if !errors.Is(actual.err, sourcegate.ErrSourcePaused) || !errors.Is(err, sourcegate.ErrSourcePaused) {
						t.Fatalf("actual=%v sequential=%v", actual.err, err)
					}
					return
				}
				if actual.err != nil || err != nil || !reflect.DeepEqual(actual.result, expected) || !reflect.DeepEqual(actual.refs, refs) {
					t.Fatalf("actual=%v sequential=%v", actual.err, err)
				}
				mu.Lock()
				n := seen[3]
				mu.Unlock()
				if n != 2 {
					t.Fatalf("page 3 calls=%d, want once per pass", n)
				}
			})
		})
	}
}

func TestCentralPass_UnknownTotalUsesSequentialWindow(t *testing.T) {
	dir := t.TempDir()
	synctest.Test(t, func(t *testing.T) {
		var mu sync.Mutex
		seen := map[int]int{}
		release := make(chan struct{})
		transport := centralTransport(func(r *http.Request) (*http.Response, error) {
			var q struct {
				Page int `json:"pageNumber"`
			}
			if err := json.NewDecoder(r.Body).Decode(&q); err != nil {
				return nil, err
			}
			mu.Lock()
			seen[q.Page]++
			mu.Unlock()
			if q.Page == 2 {
				<-release
			}
			body := listingPage(nil, 0, 0)
			if q.Page == 1 {
				body = listingPage(nil, 0, 1)
			}
			return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
		})
		client := vbpl.NewWithBase("https://example.test", "test", vbpl.WithTransport(transport), vbpl.WithLimits(float64(rate.Inf), 16))
		done := make(chan error, 1)
		go func() {
			_, _, err := New(client).centralPass(context.Background(), dir, 1, "test", json.RawMessage(`{}`))
			done <- err
		}()
		synctest.Wait()
		mu.Lock()
		early := seen[3]
		mu.Unlock()
		close(release)
		err := <-done
		if early != 0 {
			t.Errorf("page 3 was requested before page 2 completed")
		}
		if err != nil {
			t.Fatal(err)
		}
	})
}
