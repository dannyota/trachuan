package chinhphu

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"syscall"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/sourcegate"
)

type budgetReader struct {
	c *Client
	r io.Reader
}

func (b budgetReader) Read(p []byte) (int, error) {
	b.c.budgetMu.Lock()
	defer b.c.budgetMu.Unlock()
	remain := b.c.budget - b.c.used
	if remain <= 0 {
		return 0, ErrBudget
	}
	if int64(len(p)) > remain {
		p = p[:remain]
	}
	n, e := b.r.Read(p)
	b.c.used += int64(n)
	return n, e
}
func (c *Client) admit(ctx context.Context) error {
	if c.gate == nil {
		return nil
	}
	id, ok := sourcegate.RunID(ctx)
	if !ok {
		return errors.New("source admission: missing run id")
	}
	if e := c.gate.Admit(ctx, id, "chinhphu"); e != nil {
		return fmt.Errorf("source admission: %w", e)
	}
	return nil
}
func (c *Client) pause(ctx context.Context, reason string) error {
	if c.gate != nil {
		id, ok := sourcegate.RunID(ctx)
		if !ok {
			return errors.New("source pause: missing run id")
		}
		if e := c.gate.Pause(ctx, id, "chinhphu", reason, ""); e != nil {
			return fmt.Errorf("source pause: %w", e)
		}
	}
	return fmt.Errorf("source denied access: %w", sourcegate.ErrSourcePaused)
}
func retryDelay(h string, now time.Time) time.Duration {
	if h == "" {
		return 0
	}
	value := strings.TrimSpace(h)
	if value != "" && strings.Trim(value, "0123456789") == "" {
		n, e := strconv.ParseUint(value, 10, 64)
		if e != nil || n > 600 {
			return 601 * time.Second
		}
		return time.Duration(n) * time.Second
	}
	if t, e := http.ParseTime(h); e == nil && t.After(now) {
		return t.Sub(now)
	}
	return 0
}
func classified(err error, endpoint string, status int) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, sourcegate.ErrSourcePaused) {
		return err
	}
	cause, origin := "unknown", "transport"
	var n net.Error
	switch {
	case errors.Is(err, ErrTooLarge), errors.Is(err, ErrBudget), errors.Is(err, errInvalidPayload):
		cause = "invalid_payload"
		origin = "source_data"
	case errors.Is(err, context.DeadlineExceeded), errors.As(err, &n) && n.Timeout():
		cause = "timeout"
	case errors.As(err, &n), errors.Is(err, io.ErrUnexpectedEOF), errors.Is(err, syscall.ECONNRESET), errors.Is(err, syscall.ECONNREFUSED):
		cause = "connection_error"
	}
	var safe error
	for _, known := range []error{ErrTooLarge, ErrBudget, errInvalidPayload, context.Canceled, context.DeadlineExceeded} {
		if errors.Is(err, known) {
			safe = known
			break
		}
	}
	return &Failure{Origin: origin, Cause: cause, Endpoint: endpoint, HTTPStatus: status, Err: safe}
}
func (c *Client) request(ctx context.Context, method, target string, form url.Values, file bool, consume func(io.Reader, http.Header) error) error {
	endpoint := "page"
	timeout := c.limits.PageTimeout
	if file {
		endpoint = "attachment"
		timeout = c.limits.FileTimeout
	}
	for attempt := 0; ; attempt++ {
		status, after, err := c.attempt(ctx, method, target, form, file, timeout, consume)
		if err != nil && errors.Is(err, sourcegate.ErrSourcePaused) {
			return err
		}
		if errors.Is(err, ErrBudget) || errors.Is(err, ErrTooLarge) {
			return classified(err, endpoint, status)
		}
		var ne net.Error
		var partial *partialDownload
		retry := status == 429 || status >= 500 || errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &ne) && ne.Timeout())
		if errors.As(err, &partial) {
			retry = false
		}
		if retry && attempt < len(c.backoffs) {
			delay := c.backoffs[attempt]
			if after > delay {
				delay = after
			}
			if delay > 10*time.Minute {
				return &Failure{Origin: "gateway", Cause: statusCause(status), Endpoint: endpoint, HTTPStatus: status, Err: ErrDeferred}
			}
			if e := c.sleep(ctx, delay); e != nil {
				return classified(e, endpoint, status)
			}
			continue
		}
		if retry && after > 10*time.Minute {
			return &Failure{Origin: "gateway", Cause: statusCause(status), Endpoint: endpoint, HTTPStatus: status, Err: ErrDeferred}
		}
		if err != nil {
			return classified(err, endpoint, status)
		}
		if status == 404 {
			return ErrNotFound
		}
		if status != 200 {
			cause := "unknown"
			if status == 429 {
				cause = "http_429"
			} else if status >= 500 {
				cause = "http_5xx"
			}
			return &Failure{Origin: "gateway", Cause: cause, Endpoint: endpoint, HTTPStatus: status}
		}
		return nil
	}
}
func (c *Client) attempt(ctx context.Context, method, target string, form url.Values, file bool, timeout time.Duration, consume func(io.Reader, http.Header) error) (int, time.Duration, error) {
	for redirects := 0; ; redirects++ {
		status, loc, after, err := c.hop(ctx, method, target, form, file, timeout, consume)
		if err != nil {
			return status, after, err
		}
		if status < 300 || status > 399 {
			return status, after, nil
		}
		if redirects >= c.limits.Redirects {
			return status, 0, errors.New("redirect limit exceeded")
		}
		u, e := url.Parse(target)
		if e != nil {
			return status, 0, errors.New("invalid redirect")
		}
		next, e := u.Parse(loc)
		if e != nil {
			return status, 0, errors.New("invalid redirect")
		}
		target = next.String()
		if status == 301 || status == 302 || status == 303 {
			method = http.MethodGet
			form = nil
		}
	}
}
func (c *Client) hop(ctx context.Context, method, target string, form url.Values, file bool, timeout time.Duration, consume func(io.Reader, http.Header) error) (status int, location string, delay time.Duration, err error) {
	started := time.Now()
	name := "page"
	if file {
		name = "attachment"
	}
	defer func() { sourcegate.LogSlow(ctx, "chinhphu", name, time.Since(started), 10*time.Second, status, err) }()
	select {
	case c.slots <- struct{}{}:
	case <-ctx.Done():
		return 0, "", 0, ctx.Err()
	}
	defer func() { <-c.slots }()
	if err = c.limiter.Wait(ctx); err != nil {
		return 0, "", 0, err
	}
	if err = c.admit(ctx); err != nil {
		return 0, "", 0, err
	}
	requestCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if err = c.checkURL(requestCtx, target, file); err != nil {
		return 0, "", 0, err
	}
	limit := c.limits.HTMLBytes
	if file {
		limit = c.limits.FileBytes
	}
	return c.one(requestCtx, method, target, form, timeout, limit, consume)
}
func (c *Client) one(ctx context.Context, method, target string, form url.Values, timeout time.Duration, limit int64, consume func(io.Reader, http.Header) error) (int, string, time.Duration, error) {
	reqctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req, e := http.NewRequestWithContext(reqctx, method, target, body)
	if e != nil {
		return 0, "", 0, errors.New("invalid request")
	}
	req.Header.Set("User-Agent", sourcegate.UserAgent)
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	resp, e := c.http.Do(req)
	if e != nil {
		var ue *url.Error
		if errors.As(e, &ue) {
			e = ue.Err
		}
		return 0, "", 0, e
	}
	defer func() { _ = resp.Body.Close() }()
	r := budgetReader{c, resp.Body}
	status := resp.StatusCode
	after := retryDelay(resp.Header.Get("Retry-After"), c.now())
	if status == 403 {
		return status, "", after, c.pause(ctx, "http 403")
	}
	if status != 200 {
		n, readErr := io.Copy(io.Discard, io.LimitReader(r, limit+1))
		if readErr != nil {
			return status, "", after, readErr
		}
		if n > limit {
			return status, "", after, ErrTooLarge
		}
		return status, resp.Header.Get("Location"), after, nil
	}
	e = consume(r, resp.Header)
	return status, "", after, e
}

func statusCause(status int) string {
	if status == 429 {
		return "http_429"
	}
	if status >= 500 {
		return "http_5xx"
	}
	return "unknown"
}
