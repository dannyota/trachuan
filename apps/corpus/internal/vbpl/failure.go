package vbpl

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"strings"
	"syscall"
)

// Failure describes a gateway or transport failure without response contents.
type Failure struct {
	Origin, Cause, Endpoint string
	HTTPStatus              int
	Err                     error
}

// Error describes the classified failure.
func (e *Failure) Error() string {
	return fmt.Sprintf("%s: %s (status %d)", e.Endpoint, e.Cause, e.HTTPStatus)
}

// Unwrap returns the underlying error, when present.
func (e *Failure) Unwrap() error { return e.Err }

func isNonJSON(err error) bool {
	var f *Failure
	return errors.As(err, &f) && f.Cause == "non_json_200"
}
func statusCause(status int) string {
	if status == 429 {
		return "http_429"
	}
	return "http_5xx"
}
func classify(err error, name string, status int) error {
	var prior *Failure
	if err == nil || errors.As(err, &prior) || isNonJSON(err) || errors.Is(err, ErrTooLarge) {
		return err
	}
	var n net.Error
	if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &n) && n.Timeout()) {
		return &Failure{Origin: "transport", Cause: "timeout", Endpoint: name, HTTPStatus: status, Err: err}
	}
	var op *net.OpError
	if errors.As(err, &op) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, syscall.ECONNRESET) {
		return &Failure{Origin: "transport", Cause: "connection_error", Endpoint: name, HTTPStatus: status, Err: err}
	}
	return err
}
func endpoint(target string) string {
	u, err := url.Parse(target)
	if err != nil {
		return "gateway"
	}
	path := u.Path
	switch {
	case strings.HasSuffix(path, "/doc/all"):
		return "doc/all"
	case strings.Contains(path, "/doc/provision/tree/"):
		return "doc/provision/tree"
	case strings.Contains(path, "/doc/minio/"):
		return "doc/files"
	case strings.HasSuffix(path, "/diagram"):
		return "doc/diagram"
	case strings.Contains(path, "/doc/"):
		return "doc/detail"
	default:
		return "attachment"
	}
}
