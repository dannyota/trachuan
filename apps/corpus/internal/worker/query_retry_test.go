package worker

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"danny.vn/trachuan/internal/embedding"
)

type scriptedEmbedder struct {
	errors      []error
	calls       int
	times       []time.Time
	recipeCalls int
}

func (s *scriptedEmbedder) Recipe(context.Context) (embedding.Recipe, error) {
	s.recipeCalls++
	return embedding.Recipe{}, errors.New("recipe failure")
}

func (s *scriptedEmbedder) Embed(_ context.Context, text string) (embedding.Vector, error) {
	s.calls++
	s.times = append(s.times, time.Now())
	if text != "query" {
		return embedding.Vector{}, errors.New("wrong text")
	}
	if s.calls <= len(s.errors) {
		return embedding.Vector{}, s.errors[s.calls-1]
	}
	return embedding.Vector{Values: []float32{1}}, nil
}

func TestRetryEmbedder_Embed(t *testing.T) {
	busy := &embedding.Error{Status: 429}
	codeBusy := &embedding.Error{Status: 503, Code: "busy"}
	one, two, thirty := time.Second, 2*time.Second, 30*time.Second
	lastBusy := &embedding.Error{Status: 429, Code: "last"}
	transport := errors.New("transport")
	for _, tt := range []struct {
		name    string
		errs    []error
		waits   []time.Duration
		wantErr error
	}{
		{"success", []error{busy, codeBusy}, []time.Duration{250 * time.Millisecond, time.Second}, nil},
		{"exhausted", []error{busy, busy, busy, lastBusy}, []time.Duration{250 * time.Millisecond, time.Second, 3 * time.Second}, lastBusy},
		{"wrapped busy", []error{fmt.Errorf("wrapped: %w", busy)}, []time.Duration{250 * time.Millisecond}, nil},
		{"shorter retry after", []error{busy, busy, &embedding.Error{Status: 429, RetryAfter: &one}}, []time.Duration{250 * time.Millisecond, time.Second, 3 * time.Second}, nil},
		{"500", []error{&embedding.Error{Status: 500}}, nil, nil},
		{"400", []error{&embedding.Error{Status: 400}}, nil, nil},
		{"transport", []error{transport}, nil, transport},
		{"retry after", []error{&embedding.Error{Status: 429, RetryAfter: &two}}, []time.Duration{2 * time.Second}, nil},
		{"retry after cap", []error{&embedding.Error{Status: 429, RetryAfter: &thirty}}, []time.Duration{5 * time.Second}, nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				fake := &scriptedEmbedder{errors: tt.errs}
				vector, err := (retryEmbedder{embedder: fake}).Embed(context.Background(), "query")
				wantErr := tt.wantErr
				if tt.name == "500" || tt.name == "400" {
					wantErr = tt.errs[0]
				}
				if err != wantErr {
					t.Fatalf("error = %v, want %v", err, wantErr)
				}
				if err == nil && !reflect.DeepEqual(vector.Values, []float32{1}) {
					t.Fatalf("vector = %v", vector)
				}
				if fake.calls != len(tt.waits)+1 {
					t.Fatalf("calls = %d, want %d", fake.calls, len(tt.waits)+1)
				}
				for i, want := range tt.waits {
					if got := fake.times[i+1].Sub(fake.times[i]); got != want {
						t.Errorf("wait %d = %v, want %v", i, got, want)
					}
				}
			})
		})
	}
}

func TestRetryEmbedder_CancelWhileWaiting(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		fake := &scriptedEmbedder{errors: []error{&embedding.Error{Status: 429}}}
		done := make(chan error, 1)
		go func() { _, err := (retryEmbedder{embedder: fake}).Embed(ctx, "query"); done <- err }()
		synctest.Wait()
		cancel()
		if err := <-done; !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %v", err)
		}
		if fake.calls != 1 {
			t.Fatalf("calls = %d", fake.calls)
		}
	})
}

func TestRetryEmbedder_RecipeDoesNotRetry(t *testing.T) {
	fake := &scriptedEmbedder{}
	_, err := (retryEmbedder{embedder: fake}).Recipe(context.Background())
	if err == nil || fake.recipeCalls != 1 {
		t.Fatalf("error = %v, calls = %d", err, fake.recipeCalls)
	}
}

type retryHeaderTransport struct {
	header string
	times  []time.Time
}

func (r *retryHeaderTransport) RoundTrip(_ *http.Request) (*http.Response, error) {
	r.times = append(r.times, time.Now())
	status := http.StatusTooManyRequests
	body := `{"error":{"code":"busy"}}`
	header := make(http.Header)
	header.Set("Retry-After", r.header)
	if len(r.times) > 1 {
		status = http.StatusOK
		body = `{"recipe_hash":"hash","dimensions":1,"data":[{"index":0,"token_count":1,"embedding":[1]}]}`
	}
	return &http.Response{StatusCode: status, Header: header, Body: io.NopCloser(strings.NewReader(body))}, nil
}

func TestRetryEmbedder_HTTPRetryAfter(t *testing.T) {
	for _, tt := range []struct {
		name   string
		offset time.Duration
		header string
		want   time.Duration
	}{
		{"future date capped", 10 * time.Second, "", 5 * time.Second},
		{"future date", 2 * time.Second, "", 2 * time.Second},
		{"past date", -time.Second, "", 250 * time.Millisecond},
		{"oversized decimal", 0, "99999999999999999999", 5 * time.Second},
		{"garbage", 0, "garbage", 250 * time.Millisecond},
	} {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				header := tt.header
				if header == "" {
					header = time.Now().Add(tt.offset).UTC().Format(http.TimeFormat)
				}
				transport := &retryHeaderTransport{header: header}
				q := embedding.Query{Client: embedding.NewClient("http://embedder", &http.Client{Transport: transport}), RecipeHash: "hash"}
				_, err := (retryEmbedder{embedder: q}).Embed(context.Background(), "query")
				if err != nil {
					t.Fatal(err)
				}
				if len(transport.times) != 2 {
					t.Fatalf("calls = %d", len(transport.times))
				}
				if got := transport.times[1].Sub(transport.times[0]); got != tt.want {
					t.Fatalf("wait = %v, want %v", got, tt.want)
				}
			})
		})
	}
}
