package vbpl

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

func (c *Client) apiRaw(ctx context.Context, method, path string, body []byte) (json.RawMessage, error) {
	activityCtx := ctx
	ctx, cancel := context.WithTimeout(ctx, c.requestBudget)
	defer cancel()
	var last5xx int
	var saw429 bool
	failure := func() error {
		status := last5xx
		if saw429 {
			status = http.StatusTooManyRequests
		}
		if status == 0 {
			return nil
		}
		name := endpoint(path)
		return &Failure{Origin: "gateway", Cause: statusCause(status), Endpoint: name, HTTPStatus: status, Err: &StatusError{Code: status, Endpoint: name}}
	}
	for attempt := 0; ; attempt++ {
		if attempt > 0 {
			if err := wait(ctx, c.backoffs[attempt-1]); err != nil {
				if activityCtx.Err() != nil {
					return nil, activityCtx.Err()
				}
				if prior := failure(); prior != nil {
					return nil, prior
				}
				return nil, fmt.Errorf("retry: %w", classify(err, endpoint(path), 0))
			}
		}
		var raw []byte
		status, err := c.request(ctx, method, c.base+path, body, false, func(r io.Reader) error {
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
		if activityCtx.Err() != nil {
			return nil, activityCtx.Err()
		}
		if status == http.StatusTooManyRequests {
			saw429 = true
		} else if status >= 500 && status <= 599 {
			last5xx = status
		}
		if err == nil && status == http.StatusOK && !objectFramed(raw) {
			err = &Failure{Origin: "gateway", Cause: "non_json_200", Endpoint: endpoint(path), HTTPStatus: status}
		}
		if retryable(status, err) || isNonJSON(err) {
			if attempt < len(c.backoffs) {
				continue
			}
			if prior := failure(); prior != nil {
				return nil, prior
			}
		}
		if err != nil {
			return nil, fmt.Errorf("gateway request: %w", classify(err, endpoint(path), status))
		}
		if status == 404 {
			return nil, ErrNotFound
		}
		if status != 200 {
			statusErr := &StatusError{Code: status, Endpoint: endpoint(path)}
			if status == 429 || status >= 500 {
				return nil, &Failure{Origin: "gateway", Cause: statusCause(status), Endpoint: endpoint(path), HTTPStatus: status, Err: statusErr}
			}
			return nil, statusErr
		}
		return raw, nil
	}
}
