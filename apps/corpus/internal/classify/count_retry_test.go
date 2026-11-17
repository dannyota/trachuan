package classify

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/anthropics/anthropic-sdk-go/v2"
)

type retryCounterClient struct {
	*fakeClient
	count func(context.Context, anthropic.MessageNewParams) (int64, error)
}

func (c retryCounterClient) CountTokens(ctx context.Context, p anthropic.MessageNewParams) (int64, error) {
	return c.count(ctx, p)
}

func TestCountTokens_RetryPolicy(t *testing.T) {
	cases := []struct {
		name     string
		failures []error
		calls    int
		success  bool
	}{
		{"transport", []error{&url.Error{Op: "Post", URL: "https://example.invalid", Err: errors.New("transport")}}, 2, true},
		{"body read", []error{fmt.Errorf("error reading response body: %w", errors.New("interrupted"))}, 1, false},
		{"decode", []error{errors.New("error parsing response json")}, 1, false},
		{"wrapped server", []error{fmt.Errorf("wrapped: %w", &anthropic.Error{StatusCode: 500})}, 2, true},
		{"context cancelled", []error{context.Canceled}, 1, false},
		{"context deadline", []error{context.DeadlineExceeded}, 1, false},
		{"wrapped context deadline", []error{&url.Error{Op: "Post", URL: "https://example.invalid", Err: context.DeadlineExceeded}}, 1, false},
		{"server recovery", []error{&anthropic.Error{StatusCode: 500}, &anthropic.Error{StatusCode: 502}}, 3, true},
		{"server exhausted", []error{&anthropic.Error{StatusCode: 503}, &anthropic.Error{StatusCode: 503}, &anthropic.Error{StatusCode: 503}}, 3, false},
	}
	for _, status := range []int{400, 401, 404, 413, 429} {
		cases = append(cases, struct {
			name     string
			failures []error
			calls    int
			success  bool
		}{fmt.Sprint(status), []error{&anthropic.Error{StatusCode: status}}, 1, false})
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			count := func(context.Context, anthropic.MessageNewParams) (int64, error) {
				calls++
				if calls <= len(tc.failures) {
					return 0, tc.failures[calls-1]
				}
				return 100, nil
			}
			_, err := countTokensWithWait(context.Background(), anthropic.MessageNewParams{}, count, func(context.Context, time.Duration) error { return nil })
			if calls != tc.calls || (err == nil) != tc.success {
				t.Fatalf("calls %d, error %v", calls, err)
			}
		})
	}
}

func TestCountTokens_CancellationStopsRetry(t *testing.T) {
	for _, path := range []string{"screen", "articles"} {
		t.Run(path, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			calls := 0
			err := runCountPath(t, path, ctx, func(context.Context, anthropic.MessageNewParams) (int64, error) {
				calls++
				cancel()
				return 0, &url.Error{Op: "Post", URL: "https://example.invalid", Err: errors.New("transport")}
			})
			if calls != 1 || !errors.Is(err, context.Canceled) {
				t.Fatalf("calls %d, error %v", calls, err)
			}
		})
	}
}

func runCountPath(t *testing.T, path string, ctx context.Context, count TokenCounter) error {
	t.Helper()
	if path == "screen" {
		r, _, client, o := screenFixture(t)
		r.Client = retryCounterClient{client, count}
		_, err := r.prepareScreenBatch(ctx, o, documents()[:1], families())
		return err
	}
	r := articleRecipe(t)
	_, err := BuildArticlePlan(ctx, r, articleInputs(1, ""), map[string]Outcome{"vn:vbpl:1": {Outcome: "ok", Codes: []string{"1.1"}}}, articleNodes(), count)
	return err
}

func TestCountTokens_CallersKeepProviderSummary(t *testing.T) {
	for _, path := range []string{"screen", "articles"} {
		t.Run(path, func(t *testing.T) {
			calls := 0
			err := runCountPath(t, path, context.Background(), func(context.Context, anthropic.MessageNewParams) (int64, error) {
				calls++
				return 0, &anthropic.Error{StatusCode: 429}
			})
			if calls != 1 || err == nil || !strings.Contains(err.Error(), "status=429") {
				t.Fatalf("calls %d error %v", calls, err)
			}
		})
	}
}

func TestCountTokens_UsesBackoffAndStopsWhenWaitCancelled(t *testing.T) {
	t.Run("delays", func(t *testing.T) {
		calls := 0
		var delays []time.Duration
		n, err := countTokensWithWait(context.Background(), anthropic.MessageNewParams{}, func(context.Context, anthropic.MessageNewParams) (int64, error) {
			calls++
			if calls < 3 {
				return 0, &anthropic.Error{StatusCode: 503}
			}
			return 100, nil
		}, func(_ context.Context, d time.Duration) error { delays = append(delays, d); return nil })
		if err != nil || n != 100 || calls != 3 || len(delays) != 2 || delays[0] != time.Second || delays[1] != 2*time.Second {
			t.Fatalf("count %d calls %d delays %v error %v", n, calls, delays, err)
		}
	})
	t.Run("cancel", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		calls := 0
		_, err := countTokensWithWait(ctx, anthropic.MessageNewParams{}, func(context.Context, anthropic.MessageNewParams) (int64, error) {
			calls++
			return 0, &anthropic.Error{StatusCode: 503}
		}, func(ctx context.Context, _ time.Duration) error { cancel(); return ctx.Err() })
		if calls != 1 || !errors.Is(err, context.Canceled) {
			t.Fatalf("calls %d error %v", calls, err)
		}
	})
}
