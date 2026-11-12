package vbpl

import (
	"errors"
	"io"
)

// responseBody separates network read errors from local writer errors.
type responseBody struct {
	reader   io.Reader
	endpoint string
	status   int
}

func (r responseBody) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	if err == nil || errors.Is(err, io.EOF) {
		return n, err
	}
	classified := classify(err, r.endpoint, r.status)
	var failure *Failure
	if errors.As(classified, &failure) {
		return n, classified
	}
	return n, &Failure{Origin: "transport", Cause: "connection_error", Endpoint: r.endpoint, HTTPStatus: r.status, Err: err}
}
