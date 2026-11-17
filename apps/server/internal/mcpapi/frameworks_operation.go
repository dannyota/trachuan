package mcpapi

import (
	"context"
	"encoding/json"

	"danny.vn/trachuan/internal/frameworks/serve"
)

type FrameworkOperation func(context.Context, int64, json.RawMessage, string) (json.RawMessage, error)

func NewFrameworkOperation(cfg Config, name string) FrameworkOperation {
	input, output := frameworkSchemas(name)
	resolvedInput, err := input.Resolve(nil)
	if err != nil {
		panic("invalid framework input schema")
	}
	resolvedOutput, err := output.Resolve(nil)
	if err != nil {
		panic("invalid framework output schema")
	}
	return func(ctx context.Context, accountID int64, raw json.RawMessage, expectedHandle string) (json.RawMessage, error) {
		var value any
		if !cfg.Frameworks || json.Unmarshal(raw, &value) != nil || resolvedInput.Validate(value) != nil {
			return nil, serve.ErrInvalidRequest
		}
		out, err := cfg.frameworkRequest(ctx, accountID, name, raw, expectedHandle)
		if err != nil {
			return nil, err
		}
		result, data, err := encodeResult(out, false)
		if err != nil || resolvedOutput.Validate(data) != nil {
			return nil, serve.ErrUnavailable
		}
		return result.StructuredContent.(json.RawMessage), nil
	}
}
