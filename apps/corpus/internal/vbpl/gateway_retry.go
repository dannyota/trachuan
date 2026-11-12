package vbpl

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"danny.vn/trachuan/apps/corpus/internal/sourcegate"
)

func (c *Client) apiRaw(ctx context.Context, method, path string, body []byte) (json.RawMessage, error) {
	policy := sourcegate.Retry{Backoffs: c.backoffs, WaitLimit: c.requestBudget}
	for {
		var raw []byte
		var headers http.Header
		status, err := c.request(ctx, method, c.base+path, body, false, requestOptions{headers: &headers}, func(r io.Reader) error {
			var e error
			raw, e = io.ReadAll(io.LimitReader(r, apiLimit+1))
			if e != nil {
				return e
			}
			if int64(len(raw)) > apiLimit {
				return ErrTooLarge
			}
			return nil
		})
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if err == nil && status == 200 && !json.Valid(raw) {
			err = &Failure{Origin: "gateway", Cause: "non_json_200", Endpoint: endpoint(path), HTTPStatus: status}
		}
		delay, retry, deferred := policy.Next(status, retryable(status, err) || isNonJSON(err), sourcegate.RetryAfter(headers.Get("Retry-After"), c.now()))
		if deferred {
			return nil, &Failure{Origin: "gateway", Cause: "unknown", Err: ErrDeferred, Endpoint: endpoint(path), HTTPStatus: status}
		}
		if retry {
			if e := c.sleep(ctx, delay); e != nil {
				return nil, e
			}
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("gateway request: %w", classify(err, endpoint(path), status))
		}
		if status == 404 {
			return nil, &Failure{Origin: "gateway", Cause: "unknown", Endpoint: endpoint(path), HTTPStatus: 404, Err: ErrNotFound}
		}
		if status != 200 {
			return nil, statusFailure(status, endpoint(path))
		}
		return raw, nil
	}
}

func statusFailure(status int, name string) error {
	cause := "unknown"
	if status == 429 || status >= 500 && status <= 599 {
		cause = statusCause(status)
	}
	return &Failure{Origin: "gateway", Cause: cause, Endpoint: name, HTTPStatus: status, Err: &StatusError{Code: status, Endpoint: name}}
}
