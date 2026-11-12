package mcpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"math"

	"danny.vn/trachuan/apps/server/internal/apierror"
	"danny.vn/trachuan/apps/server/internal/release"
	"danny.vn/trachuan/apps/server/internal/reports"
	"danny.vn/trachuan/internal/retrieval"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type invalidRequest struct{}

func (invalidRequest) Error() string { return "invalid_request" }

type errorResult struct {
	Error toolError `json:"error"`
}
type toolError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// ErrorMessage maps tool failures to fixed public codes and messages.
func ErrorMessage(err error) (string, string) {
	var invalid invalidRequest
	var report reports.Error
	if delay, ok := reports.RetryDelay(err); ok {
		seconds := max(1, int(math.Ceil(delay.Seconds())))
		return "rate_limited", apierror.RetryMessage(seconds)
	}
	if errors.As(err, &report) {
		switch report {
		case reports.ErrInvalidRequest, reports.ErrRateLimited, reports.ErrReleaseExpired, reports.ErrUnavailable, reports.ErrUnauthenticated:
			return string(report), apierror.Message(string(report))
		}
	}
	switch {
	case errors.As(err, &invalid), errors.Is(err, retrieval.ErrInvalidRequest):
		return "invalid_request", apierror.Message("invalid_request")
	case errors.Is(err, retrieval.ErrNotFound):
		return "not_found", apierror.Message("not_found")
	case errors.Is(err, release.ErrExpired), errors.Is(err, retrieval.ErrReleaseGone), errors.Is(err, retrieval.ErrReleaseExpired):
		return "release_expired", apierror.Message("release_expired")
	case errors.Is(err, retrieval.ErrEmbedderBusy):
		return "busy", apierror.Message("busy")
	case errors.Is(err, retrieval.ErrEmbedder):
		return "inference_failed", apierror.Message("inference_failed")
	default:
		return "unavailable", apierror.Message("unavailable")
	}
}
func toolFailure(code, message string) *mcp.CallToolResult {
	result, _, _ := encodeResult(errorResult{Error: toolError{Code: code, Message: message}}, true)
	return result
}
func encodeResult(value any, isError bool) (*mcp.CallToolResult, any, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, nil, err
	}
	var data any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err = decoder.Decode(&data); err != nil {
		return nil, nil, err
	}
	omitEmpty(data)
	raw, err = json.Marshal(data)
	if err != nil {
		return nil, nil, err
	}
	// The schema library expects ordinary decoded numbers. Keep its validation
	// view separate so large integer counts stay exact in both wire copies.
	var validation any
	if err = json.Unmarshal(raw, &validation); err != nil {
		return nil, nil, err
	}
	return &mcp.CallToolResult{IsError: isError, StructuredContent: json.RawMessage(raw), Content: []mcp.Content{&mcp.TextContent{Text: string(raw)}}}, validation, nil
}
func omitEmpty(value any) {
	switch v := value.(type) {
	case map[string]any:
		for k, item := range v {
			omitEmpty(item)
			empty := item == nil
			switch x := item.(type) {
			case string:
				empty = x == ""
			case []any:
				empty = len(x) == 0
			case map[string]any:
				empty = len(x) == 0
			}
			if empty {
				delete(v, k)
			}
		}
	case []any:
		for _, item := range v {
			omitEmpty(item)
		}
	}
}

func errorMessage(err error) (string, string) { return ErrorMessage(err) }
