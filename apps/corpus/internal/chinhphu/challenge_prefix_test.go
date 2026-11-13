package chinhphu

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/sourcegate"
)

func TestChallengePrefix_SplitPageWithForm(t *testing.T) {
	tr := roundTripFunc(func(*http.Request) (*http.Response, error) {
		body := io.MultiReader(strings.NewReader(`<html><script src="captcha.js"></script>`), strings.NewReader(`<form></form></html>`))
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(body)}, nil
	})
	c := New(WithBaseURLs("https://test.invalid", "https://test.invalid"), WithTransport(tr), WithBackoffs())
	page, _, err := c.fetchPage(sourcegate.WithActivity(context.Background()), http.MethodGet, "https://test.invalid", nil)
	if err != nil || !strings.Contains(string(page), "<form>") {
		t.Fatalf("page=%s error=%v", page, err)
	}
}

func TestChallengePrefix_PositiveEvidenceStopsEarly(t *testing.T) {
	for _, marker := range []string{"cf-chl", "challenge-platform"} {
		t.Run(marker, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				tr := roundTripFunc(func(r *http.Request) (*http.Response, error) {
					stalled := &stalledPrefix{ctx: r.Context(), head: []byte(marker + `</div>`), closed: make(chan struct{})}
					return &http.Response{StatusCode: 200, Header: http.Header{}, Body: structReadCloser{Reader: io.MultiReader(strings.NewReader(`<html><div>`), stalled), Closer: stalled}}, nil
				})
				c := New(WithBaseURLs("https://test.invalid", "https://test.invalid"), WithTransport(tr), WithBackoffs())
				start := time.Now()
				_, _, err := c.fetchPage(sourcegate.WithActivity(context.Background()), http.MethodGet, "https://test.invalid", nil)
				if !errors.Is(err, sourcegate.ErrSourcePaused) || time.Since(start) != 0 {
					t.Fatalf("elapsed=%s error=%v", time.Since(start), err)
				}
			})
		})
	}
}

type structReadCloser struct {
	io.Reader
	io.Closer
}

func TestChallengePrefix_CompleteCaptchaWithoutForm(t *testing.T) {
	for _, tc := range []struct{ name, body string }{
		{"body", `<html>captcha</html>`},
		{"limit", `<html>captcha` + strings.Repeat("x", 4096)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tr := roundTripFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: 404, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(tc.body))}, nil
			})
			c := New(WithBaseURLs("https://test.invalid", "https://test.invalid"), WithTransport(tr), WithBackoffs())
			_, _, err := c.fetchPage(sourcegate.WithActivity(context.Background()), http.MethodGet, "https://test.invalid", nil)
			if !errors.Is(err, sourcegate.ErrSourcePaused) {
				t.Fatalf("error=%v", err)
			}
		})
	}
}

func TestChallengePrefix_InterruptedCaptchaIsNotComplete(t *testing.T) {
	tr := roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(io.MultiReader(strings.NewReader(`<html>captcha`), brokenBody{}))}, nil
	})
	c := New(WithBaseURLs("https://test.invalid", "https://test.invalid"), WithTransport(tr), WithBackoffs())
	_, _, err := c.fetchPage(sourcegate.WithActivity(context.Background()), http.MethodGet, "https://test.invalid", nil)
	if err == nil || errors.Is(err, sourcegate.ErrSourcePaused) {
		t.Fatalf("error=%v", err)
	}
}
