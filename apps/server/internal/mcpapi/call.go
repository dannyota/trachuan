package mcpapi

import (
	"context"
	"encoding/json"

	"danny.vn/trachuan/internal/retrieval"
	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func toolCall[In, Out any](cfg Config, name string, run func(context.Context, In) (Out, error)) (*jsonschema.Schema, *jsonschema.Schema, func(context.Context, json.RawMessage) (*mcp.CallToolResult, error)) {
	var input *jsonschema.Schema
	if name == "report_issue" {
		input = reportSchema()
	} else {
		input = schemaFor[In]()
	}
	input.Properties["country"].Enum = []any{"vn"}
	if name == "search" {
		input.Properties["detail"].Enum = []any{"compact", "full"}
		input.Properties["as_of"].MinLength = ptrInt(10)
		input.Properties["as_of"].MaxLength = ptrInt(10)
		input.Properties["limit"].Minimum = ptrFloat(1)
		input.Properties["limit"].Maximum = ptrFloat(retrieval.MaxLimit)
		input.Properties["limit"].Types = nil
		input.Properties["limit"].Type = "integer"
		input.Properties["in_force_only"].Types = nil
		input.Properties["in_force_only"].Type = "boolean"
	}
	if name == "read" {
		input.Properties["items"].MaxItems = ptrInt(retrieval.ReadItemsMax)
		input.Properties["items"].Types = nil
		input.Properties["items"].Type = "array"
	}
	resolved, err := input.Resolve(nil)
	if err != nil {
		panic("invalid tool input schema")
	}
	output := schemaFor[Out]()
	if name != "report_issue" {
		optionalEmptyFields(output)
	}
	output = &jsonschema.Schema{Type: "object", AnyOf: []*jsonschema.Schema{output, schemaFor[errorResult]()}}
	outputResolved, err := output.Resolve(nil)
	if err != nil {
		panic("invalid tool output schema")
	}
	return input, output, func(ctx context.Context, raw json.RawMessage) (*mcp.CallToolResult, error) {
		var value any
		if len(raw) == 0 || json.Unmarshal(raw, &value) != nil || resolved.Validate(value) != nil {
			return nil, invalidRequest{}
		}
		var in In
		if json.Unmarshal(raw, &in) != nil {
			return nil, invalidRequest{}
		}
		if name == "search" && cfg.Caches != nil {
			result, data, err := cfg.cachedSearch(ctx, any(in).(searchInput), func(v any) error { return outputResolved.Validate(v) })
			if err != nil {
				return nil, err
			}
			if outputResolved.Validate(data) != nil {
				return nil, retrieval.ErrUnavailable
			}
			return result, nil
		}
		out, err := run(ctx, in)
		if err != nil {
			return nil, err
		}
		result, data, err := encodeResult(out, false)
		if err != nil || outputResolved.Validate(data) != nil {
			return nil, retrieval.ErrUnavailable
		}
		return result, nil
	}
}
