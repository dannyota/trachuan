package ocrvision

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

func fakeClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return NewClient(Config{BaseURL: server.URL, Project: "private-project", TokenSource: oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "private-token"}), Sleep: func(context.Context, time.Duration) error { return nil }, Wait: func(context.Context) error { return nil }})
}

func TestClient_RequestAndRetries(t *testing.T) {
	var calls atomic.Int64
	var sleeps []time.Duration
	c := fakeClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != "POST" || r.URL.Path != "/v1/images:annotate" || r.Header.Get("Authorization") != "Bearer private-token" || r.Header.Get("x-goog-user-project") != "private-project" || r.Header.Get("Content-Type") != "application/json" {
			t.Error("wrong request metadata")
		}
		var body struct {
			Requests []struct {
				Image        struct{ Content string }
				Features     []struct{ Type, Model string }
				ImageContext struct{ LanguageHints []string }
			}
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if len(body.Requests) != 1 {
			t.Error("expected exactly one image")
			return
		}
		q := body.Requests[0]
		if q.Image.Content != base64.StdEncoding.EncodeToString([]byte("PNG")) || len(q.Features) != 1 || q.Features[0].Type != "DOCUMENT_TEXT_DETECTION" || q.Features[0].Model != "builtin/latest" || strings.Join(q.ImageContext.LanguageHints, ",") != "vi" {
			t.Error("wrong request body")
		}
		if calls.Load() < 4 {
			w.WriteHeader(503)
			return
		}
		_, _ = io.WriteString(w, `{"responses":[{"fullTextAnnotation":{"text":"text"}}]}`)
	})
	c.sleep = func(_ context.Context, d time.Duration) error { sleeps = append(sleeps, d); return nil }
	b := newBudget(4)
	got, err := c.recognize(context.Background(), []byte("PNG"), b)
	if err != nil || got != "text" || calls.Load() != 4 || fmt.Sprint(sleeps) != "[1s 4s 16s]" || b.used() != 4 {
		t.Fatalf("text=%q err=%v calls=%d sleeps=%v", got, err, calls.Load(), sleeps)
	}
}

func TestClient_FailuresAndBudget(t *testing.T) {
	for _, tc := range []struct {
		name, body   string
		status, want int
	}{
		{"empty response", `{"responses":[]}`, 200, 1},
		{"several responses", `{"responses":[{},{}]}`, 200, 1},
		{"invalid JSON", `private-token`, 200, 1},
		{"permanent status", `private-token`, 403, 1},
		{"page error", `{"responses":[{"error":{"code":3,"message":"private-token private-project"}}]}`, 200, 1},
		{"deadline", `{"responses":[{"error":{"code":4}}]}`, 200, 4},
		{"resource", `{"responses":[{"error":{"code":8}}]}`, 200, 4},
		{"internal", `{"responses":[{"error":{"code":13}}]}`, 200, 4},
		{"unavailable", `{"responses":[{"error":{"code":14}}]}`, 200, 4},
		{"429", "", 429, 4}, {"500", "", 500, 4}, {"502", "", 502, 4}, {"503", "", 503, 4}, {"504", "", 504, 4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int64
			c := fakeClient(t, func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			})
			_, err := c.recognize(context.Background(), []byte("PNG"), newBudget(10))
			if err == nil || strings.Contains(err.Error(), "private-") || int(calls.Load()) != tc.want {
				t.Fatalf("err=%v calls=%d", err, calls.Load())
			}
		})
	}
	var calls atomic.Int64
	c := fakeClient(t, func(w http.ResponseWriter, _ *http.Request) { calls.Add(1); w.WriteHeader(503) })
	_, err := c.recognize(context.Background(), nil, newBudget(1))
	if err == nil || calls.Load() != 1 {
		t.Fatal("retry exceeded budget")
	}
}

func TestClient_BudgetAtomic(t *testing.T) {
	var calls atomic.Int64
	c := fakeClient(t, func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		_, _ = io.WriteString(w, `{"responses":[{}]}`)
	})
	b := newBudget(17)
	var wg sync.WaitGroup
	for range 100 {
		wg.Go(func() { _, _ = c.recognize(context.Background(), nil, b) })
	}
	wg.Wait()
	if calls.Load() != 17 || b.used() != 17 {
		t.Fatalf("calls=%d debits=%d", calls.Load(), b.used())
	}
}

func TestClient_NoSecretsInLogs(t *testing.T) {
	var logs strings.Builder
	old := log.Writer()
	log.SetOutput(&logs)
	defer log.SetOutput(old)
	c := fakeClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"responses":[{"error":{"code":3,"message":"private-token private-project"}}]}`)
	})
	_, err := c.recognize(context.Background(), nil, newBudget(1))
	if err == nil || strings.Contains(err.Error()+logs.String(), "private-") {
		t.Fatal("unsafe error or logs")
	}
}
