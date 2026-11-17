package mcpapi

import (
	"context"
	"encoding/json"

	"danny.vn/trachuan/internal/frameworks/serve"
	"danny.vn/trachuan/internal/retrieval"
	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type frameworkInput struct {
	Corpus  string `json:"corpus"`
	Release string `json:"release,omitempty"`
}

type frameworkSearchInput struct {
	frameworkInput
	Query      string   `json:"query"`
	Frameworks []string `json:"frameworks,omitempty"`
	Versions   []string `json:"versions,omitempty"`
	Limit      int      `json:"limit,omitempty"`
	Cursor     string   `json:"cursor,omitempty"`
}

type frameworkReadInput struct {
	frameworkInput
	Items []serve.ReadItem `json:"items"`
}

type frameworkGapsInput struct {
	frameworkInput
	Versions []string `json:"versions,omitempty"`
	Kind     string   `json:"kind,omitempty"`
	Cursor   string   `json:"cursor,omitempty"`
}

type frameworkMappingsInput struct {
	frameworkInput
	Handle   string   `json:"handle,omitempty"`
	From     string   `json:"from,omitempty"`
	To       string   `json:"to,omitempty"`
	Relation []string `json:"relation,omitempty"`
	Limit    int      `json:"limit,omitempty"`
	Cursor   string   `json:"cursor,omitempty"`
}

func frameworkSchemas(name string) (*jsonschema.Schema, *jsonschema.Schema) {
	var input, output *jsonschema.Schema
	switch name {
	case "corpus_status":
		input, output = schemaFor[frameworkInput](), schemaFor[serve.StatusResponse]()
	case "search":
		input, output = schemaFor[frameworkSearchInput](), schemaFor[serve.Response]()
	case "read":
		input, output = schemaFor[frameworkReadInput](), schemaFor[serve.ReadResponse]()
		input.Properties["items"].MaxItems = ptrInt(retrieval.ReadItemsMax)
		input.Properties["items"].Type = "array"
		input.Properties["items"].Types = nil
	case "quality_gaps":
		input, output = schemaFor[frameworkGapsInput](), schemaFor[serve.GapsResponse]()
	case "mappings":
		input, output = schemaFor[frameworkMappingsInput](), schemaFor[serve.MappingsResponse]()
	default:
		return nil, nil
	}
	input.Properties["corpus"].Enum = []any{"frameworks"}
	for _, filter := range []string{"frameworks", "versions", "relation"} {
		if p := input.Properties[filter]; p != nil {
			p.Type, p.Types = "array", nil
		}
	}
	if name == "mappings" {
		input.OneOf = []*jsonschema.Schema{
			{Required: []string{"handle"}, Not: &jsonschema.Schema{AnyOf: []*jsonschema.Schema{{Required: []string{"from"}}, {Required: []string{"to"}}}}},
			{Required: []string{"from", "to"}, Not: &jsonschema.Schema{Required: []string{"handle"}}},
		}
	}
	if p := input.Properties["limit"]; p != nil {
		p.Minimum = ptrFloat(1)
		p.Maximum = ptrFloat(retrieval.MaxLimit)
		if name == "mappings" {
			p.Maximum = ptrFloat(200)
		}
	}
	if p := input.Properties["cursor"]; p != nil {
		p.MaxLength = ptrInt(2048)
	}
	optionalEmptyFields(output)
	return input, output
}

func toolCall[In, Out any](cfg Config, name string, run func(context.Context, In) (Out, error)) (*jsonschema.Schema, *jsonschema.Schema, func(context.Context, json.RawMessage) (*mcp.CallToolResult, error)) {
	input, output, lawCall := lawToolCall(cfg, name, run)
	if !cfg.Frameworks {
		return input, output, lawCall
	}
	return frameworkToolCall(cfg, name, input, output, lawCall)
}

func frameworkToolCall(cfg Config, name string, input, output *jsonschema.Schema, lawCall func(context.Context, json.RawMessage) (*mcp.CallToolResult, error)) (*jsonschema.Schema, *jsonschema.Schema, func(context.Context, json.RawMessage) (*mcp.CallToolResult, error)) {
	frameworkInput, frameworkOutput := frameworkSchemas(name)
	if frameworkInput == nil {
		return input, output, lawCall
	}
	combinedInput := &jsonschema.Schema{Type: "object", AnyOf: []*jsonschema.Schema{input, frameworkInput}}
	combinedOutput := &jsonschema.Schema{Type: "object", AnyOf: []*jsonschema.Schema{output, frameworkOutput}}
	if name == "mappings" {
		combinedInput = frameworkInput
		combinedOutput = &jsonschema.Schema{Type: "object", AnyOf: []*jsonschema.Schema{frameworkOutput, schemaFor[errorResult]()}}
	}
	resolved, err := combinedInput.Resolve(nil)
	if err != nil {
		panic("invalid combined tool input schema")
	}
	validatedOutput, err := combinedOutput.Resolve(nil)
	if err != nil {
		panic("invalid combined tool output schema")
	}
	return combinedInput, combinedOutput, func(ctx context.Context, raw json.RawMessage) (*mcp.CallToolResult, error) {
		var value map[string]any
		if json.Unmarshal(raw, &value) != nil || resolved.Validate(value) != nil {
			return nil, invalidRequest{}
		}
		if value["corpus"] != "frameworks" {
			return lawCall(ctx, raw)
		}
		out, err := cfg.frameworkCall(ctx, name, raw)
		if err != nil {
			return nil, err
		}
		result, data, err := encodeResult(out, false)
		if err != nil || validatedOutput.Validate(data) != nil {
			return nil, serve.ErrUnavailable
		}
		return result, nil
	}
}
