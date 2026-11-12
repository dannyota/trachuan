package webapi

import (
	"errors"

	"danny.vn/trachuan/apps/server/internal/mcpapi"
)

type requestError struct {
	status int
	code   string
}

func (e requestError) Error() string { return e.code }

func invalidRequest() error { return requestError{400, "invalid_request"} }

func httpError(err error) (int, string) {
	var request requestError
	if errors.As(err, &request) {
		return request.status, request.code
	}
	code, _ := mcpapi.ErrorMessage(err)
	switch code {
	case "invalid_request":
		return 400, code
	case "not_found":
		return 404, code
	case "release_expired":
		return 409, code
	case "busy":
		return 503, code
	default:
		return 503, "unavailable"
	}
}
