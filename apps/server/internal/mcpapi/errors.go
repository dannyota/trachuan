package mcpapi

import (
	"bytes"
	"encoding/json"
	"errors"

	"danny.vn/trachuan/apps/server/internal/release"
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

func errorMessage(err error) (string, string) {
	var invalid invalidRequest
	switch {
	case errors.As(err, &invalid):
		return "invalid_request", "invalid_request"
	case errors.Is(err, release.ErrExpired), errors.Is(err, retrieval.ErrReleaseGone):
		return "release_expired", "Release expired."
	case errors.Is(err, retrieval.ErrEmbedderBusy):
		return "busy", "Query embedder busy."
	case errors.Is(err, retrieval.ErrEmbedder):
		return "inference_failed", "Query inference failed."
	default:
		return "unavailable", "Service unavailable."
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
