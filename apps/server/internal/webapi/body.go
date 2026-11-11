package webapi

import (
	"encoding/json"
	"io"
	"net/http"
)

func requestBody(r *http.Request) (json.RawMessage, error) {
	const maxBodyBytes = 64 << 10
	if r.ContentLength > maxBodyBytes {
		return nil, requestError{413, "body_too_large", "Request body too large."}
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes+1))
	if len(raw) > maxBodyBytes {
		return nil, requestError{413, "body_too_large", "Request body too large."}
	}
	if err != nil {
		return nil, invalidRequest()
	}
	return json.RawMessage(raw), nil
}
