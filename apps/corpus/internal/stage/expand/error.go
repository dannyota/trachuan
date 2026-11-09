package expand

import (
	"encoding/json"
	"errors"
	"fmt"

	"danny.vn/trachuan/apps/corpus/internal/sourcegate"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/vbpl"
)

func apiError(err error, id string, detail bool) (stage.Output, error) {
	if errors.Is(err, vbpl.ErrTooLarge) {
		return stage.Output{State: "gap", Gaps: []stage.Gap{{Kind: "response_too_large", Detail: id}}}, nil
	}
	if detail && errors.Is(err, vbpl.ErrNotFound) {
		return stage.Output{State: "gap", Gaps: []stage.Gap{{Kind: "not_found", Detail: id}}}, nil
	}
	wrapped := fmt.Errorf("fetch document %q: %w", id, err)
	var syntax *json.SyntaxError
	var typeErr *json.UnmarshalTypeError
	if errors.As(err, &syntax) || errors.As(err, &typeErr) {
		return stage.Output{}, stage.SourceData(wrapped)
	}

	if errors.Is(err, sourcegate.ErrSourcePaused) {
		return stage.Output{}, stage.Retryable(wrapped)
	}
	var failure *vbpl.Failure
	if errors.As(err, &failure) {
		cause := stage.GapCause{Source: "vbpl", Origin: stage.Origin(failure.Origin), Cause: stage.Cause(failure.Cause), Endpoint: failure.Endpoint}
		if failure.HTTPStatus != 0 {
			status := int32(failure.HTTPStatus)
			cause.HTTPStatus = &status
		}
		return stage.Output{}, &stage.RetryableError{Err: wrapped, GapKind: "source_unavailable", Cause: cause}
	}
	var status *vbpl.StatusError
	if errors.As(err, &status) && (status.Code == 429 || status.Code >= 500 && status.Code <= 599) {
		code := int32(status.Code)
		cause := stage.CauseHTTP5xx
		if code == 429 {
			cause = stage.CauseHTTP429
		}
		return stage.Output{}, &stage.RetryableError{Err: wrapped, GapKind: "source_unavailable", Cause: stage.GapCause{Source: "vbpl", Origin: stage.OriginGateway, Cause: cause, Endpoint: status.Endpoint, HTTPStatus: &code}}
	}

	return stage.Output{}, stage.Retryable(wrapped)
}
