package fetch

import (
	"errors"
	"fmt"

	"danny.vn/trachuan/apps/corpus/internal/chinhphu"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/vbpl"
)

func missingFailure(err error) bool {
	if errors.Is(err, vbpl.ErrDeferred) || errors.Is(err, chinhphu.ErrDeferred) {
		return false
	}
	if errors.Is(err, vbpl.ErrNotFound) || errors.Is(err, chinhphu.ErrNotFound) {
		return true
	}
	var v *vbpl.Failure
	if errors.As(err, &v) {
		return v.Cause != "http_429"
	}
	var c *chinhphu.Failure
	return errors.As(err, &c) && c.Origin != "source_data" && c.Cause != "http_429"
}
func failureCause(err error) string {
	if errors.Is(err, vbpl.ErrNotFound) || errors.Is(err, chinhphu.ErrNotFound) {
		return "http_404"
	}
	var v *vbpl.Failure
	if errors.As(err, &v) {
		if v.Cause == "size" {
			return "size"
		}
		if v.HTTPStatus != 0 && v.HTTPStatus != 200 {
			return fmt.Sprintf("http_%d", v.HTTPStatus)
		}
		if v.Cause == "timeout" {
			return "timeout"
		}
		return "transport"
	}
	var c *chinhphu.Failure
	if errors.As(err, &c) {
		if c.HTTPStatus != 0 && c.HTTPStatus != 200 {
			return fmt.Sprintf("http_%d", c.HTTPStatus)
		}
		if c.Cause == "timeout" {
			return "timeout"
		}
		return "transport"
	}
	return "transport"
}

type attachmentConflict struct{ name string }

func (e *attachmentConflict) Error() string {
	return fmt.Sprintf("conflicting attachment evidence for %q", e.name)
}
func (e *attachmentConflict) Unwrap() error { return stage.ErrSourceData }
