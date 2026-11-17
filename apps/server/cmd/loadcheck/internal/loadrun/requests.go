package loadrun

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"math/rand/v2"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type response struct {
	status     int
	busy       bool
	retryAfter time.Duration
}

func send(ctx context.Context, cl *http.Client, method, target, body, addr, origin, csrf string, page bool, abort context.CancelFunc) response {
	req, err := http.NewRequestWithContext(ctx, method, target, strings.NewReader(body))
	if err != nil {
		return response{}
	}
	req.Header.Set("X-Forwarded-For", addr)
	if method == http.MethodPost {
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", origin)
		req.Header.Set("X-CSRF-Token", csrf)
	}
	res, err := cl.Do(req)
	if err != nil {
		return response{}
	}
	defer func() { _ = res.Body.Close() }()
	if abort != nil {
		limit := res.ContentLength
		if limit < 0 || limit > 64<<10 {
			limit = 64 << 10
		}
		if limit > 0 {
			_, _ = io.CopyN(io.Discard, res.Body, rand.Int64N(limit))
		}
		abort()
		return response{}
	}
	var b bytes.Buffer
	if _, err = io.Copy(&b, io.LimitReader(res.Body, 64<<10)); err != nil {
		return response{}
	}
	if _, err = io.Copy(io.Discard, res.Body); err != nil {
		return response{}
	}
	result := response{status: res.StatusCode, retryAfter: -1}
	if seconds, parseErr := strconv.Atoi(res.Header.Get("Retry-After")); parseErr == nil && seconds >= 0 {
		result.retryAfter = time.Duration(seconds) * time.Second
	} else if date, dateErr := http.ParseTime(res.Header.Get("Retry-After")); dateErr == nil {
		result.retryAfter = max(0, time.Until(date))
	}
	result.busy = isBusy(page, result.status, result.retryAfter, b.Bytes())
	return result
}
func keyPath(key string) string { return url.PathEscape(key) }
func abortContext(ctx context.Context, selected bool) (context.Context, context.CancelFunc) {
	if !selected {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, time.Duration(rand.Int64N(int64(100*time.Millisecond))+1))
}

func isBusy(page bool, status int, retry time.Duration, body []byte) bool {
	if page {
		return status == 503 || status == 429 && retry >= 0 && retry <= 5*time.Second
	}
	if status != 503 {
		return false
	}
	var payload struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	return json.Unmarshal(body, &payload) == nil && payload.Error.Code == "busy"
}
