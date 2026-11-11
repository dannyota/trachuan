package webapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"danny.vn/trachuan/apps/server/internal/mcpapi"
)

type requestError struct {
	status        int
	code, message string
}

func (e requestError) Error() string { return e.code }

func invalidRequest() error { return requestError{400, "invalid_request", "invalid_request"} }

func httpError(err error) (int, string, string) {
	var request requestError
	if errors.As(err, &request) {
		return request.status, request.code, request.message
	}
	code, message := mcpapi.ErrorMessage(err)
	switch code {
	case "invalid_request":
		return 400, code, message
	case "release_expired":
		return 409, code, message
	case "busy":
		return 503, code, message
	default:
		return 503, "unavailable", "Service unavailable."
	}
}

type errorResponse struct {
	Error publicError `json:"error"`
}

type publicError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	raw, _ := json.Marshal(errorResponse{Error: publicError{Code: code, Message: message}})
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(raw)
}
