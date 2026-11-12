package ocrvision

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type tokenFunc func() (*oauth2.Token, error)

func (f tokenFunc) Token() (*oauth2.Token, error) { return f() }

func TestClient_TransportRetryAndTimeout(t *testing.T) {
	c := fakeClient(t, func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, `{"responses":[{}]}`) })
	var calls int
	c.http.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		deadline, ok := r.Context().Deadline()
		if !ok || time.Until(deadline) > 120*time.Second || time.Until(deadline) < 119*time.Second {
			t.Error("missing request timeout")
		}
		if calls == 1 {
			return nil, fmt.Errorf("private-token private-project")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"responses":[{}]}`)), Header: make(http.Header)}, nil
	})
	if _, err := c.recognize(context.Background(), nil, newBudget(2)); err != nil || calls != 2 {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
}

func TestClient_PageTransientSucceeds(t *testing.T) {
	var calls atomic.Int64
	c := fakeClient(t, func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) == 1 {
			_, _ = io.WriteString(w, `{"responses":[{"error":{"code":14}}]}`)
			return
		}
		_, _ = io.WriteString(w, `{"responses":[{"fullTextAnnotation":{"text":"ok"}}]}`)
	})
	text, err := c.recognize(context.Background(), nil, newBudget(2))
	if err != nil || text != "ok" || calls.Load() != 2 {
		t.Fatalf("calls=%d text=%q err=%v", calls.Load(), text, err)
	}
}

func TestClient_RedirectDoesNotForwardCredentials(t *testing.T) {
	var calls atomic.Int64
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls.Add(1) }))
	defer target.Close()
	c := fakeClient(t, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	})
	if _, err := c.recognize(context.Background(), nil, newBudget(4)); err == nil || calls.Load() != 0 {
		t.Fatal("followed redirect")
	}
}

func TestClient_CredentialFailures(t *testing.T) {
	for _, cfg := range []Config{{}, {Project: "private-project"}, {Project: "private-project", TokenSource: tokenFunc(func() (*oauth2.Token, error) { return nil, fmt.Errorf("private-token") })}} {
		err := NewClient(cfg).Preflight(context.Background())
		if err == nil || !strings.Contains(err.Error(), "GOOGLE_APPLICATION_CREDENTIALS") || !strings.Contains(err.Error(), "GOOGLE_CLOUD_PROJECT") || strings.Contains(err.Error(), "private-") {
			t.Fatalf("unsafe authentication error: %v", err)
		}
	}
	t.Setenv("GOOGLE_CLOUD_PROJECT", "")
	if _, err := NewAuthenticated(context.Background()); err == nil {
		t.Fatal("accepted missing project")
	}
	t.Setenv("GOOGLE_CLOUD_PROJECT", "private-project")
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "/nonexistent/private-credentials")
	if _, err := NewAuthenticated(context.Background()); err == nil || strings.Contains(err.Error(), "private-") {
		t.Fatalf("missing credential file was accepted or disclosed: %v", err)
	}
}

func TestClient_ExhaustedBudgetDoesNotWait(t *testing.T) {
	c := fakeClient(t, func(http.ResponseWriter, *http.Request) { t.Error("sent without budget") })
	waits := 0
	c.wait = func(context.Context) error { waits++; return nil }
	if _, err := c.recognize(context.Background(), nil, newBudget(0)); err == nil || waits != 0 {
		t.Fatalf("waits=%d err=%v", waits, err)
	}
}
