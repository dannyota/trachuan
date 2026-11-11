// Package mcpapi serves the authenticated MCP read tools.
package mcpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"reflect"

	"log/slog"
	"net/http"
	"time"

	"danny.vn/trachuan/apps/server/internal/credential"
	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"danny.vn/trachuan/apps/server/internal/limits"
	"danny.vn/trachuan/internal/retrieval"
)

// Engine is the read-only corpus view consumed by the tools.
type Engine interface {
	Search(context.Context, retrieval.Request) (retrieval.Response, error)
	Present(context.Context, retrieval.Response, retrieval.Detail) (retrieval.SearchResult, error)
	Read(context.Context, retrieval.ReadRequest) (retrieval.ReadResult, error)
	Status(context.Context) (retrieval.CorpusStatus, error)
	Gaps(context.Context, string, int64, int) ([]retrieval.Gap, int64, error)
}

// Resolver lends an active-release engine. A successful call supplies a non-nil
// done function which the caller must invoke after all engine work finishes.
type Resolver interface {
	Resolve(context.Context, string, string) (Engine, string, func(), error)
}

// Limiter enforces per-account tool budgets.
type Limiter interface {
	Allow(int64, limits.Class) (bool, time.Duration)
}

// Config supplies read-path dependencies. Configure it before calling New.
type Config struct {
	Resolver Resolver
	Limiter  Limiter
	Logger   *slog.Logger
	Clock    func() time.Time
}

// New constructs a stateless JSON Streamable HTTP endpoint behind credential.Middleware.
func New(cfg Config) http.Handler {
	if cfg.Clock == nil {
		cfg.Clock = time.Now
	}
	s := mcp.NewServer(&mcp.Implementation{Name: "trachuan", Version: "1"}, &mcp.ServerOptions{
		Capabilities: &mcp.ServerCapabilities{Tools: &mcp.ToolCapabilities{}},
	})
	registerTools(s, cfg)
	transport := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return s }, &mcp.StreamableHTTPOptions{
		Stateless: true, JSONResponse: true, MaxRequestBodyBytes: 64 << 10, PropagateRequestCancellation: true,
	})
	return guard(transport)
}

type countryInput struct {
	Country string `json:"country"`
}

func registerTools(s *mcp.Server, cfg Config) {
	register(s, cfg, "guide", "How to search, read and cite Vietnamese law.", func(_ context.Context, _ countryInput) (guideResult, error) {
		return guideResult{Guide: guideText}, nil
	})
	register(s, cfg, "corpus_status", "Counts and coverage of the active corpus.", cfg.status)
	register(s, cfg, "quality_gaps", "Page through recorded corpus quality gaps.", cfg.gaps)
	register(s, cfg, "search", "Search Vietnamese law; compact hits by default.", cfg.search)
	register(s, cfg, "read", "Read chosen handles in one batch; follow every next token.", cfg.read)
}

// Raw handlers keep SDK schema diagnostics, which may quote input, off the wire.
func register[In, Out any](s *mcp.Server, cfg Config, name, description string, run func(context.Context, In) (Out, error)) {
	input := schemaFor[In]()
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
	optionalEmptyFields(output)
	output = &jsonschema.Schema{Type: "object", AnyOf: []*jsonschema.Schema{output, schemaFor[errorResult]()}}
	outputResolved, err := output.Resolve(nil)
	if err != nil {
		panic("invalid tool output schema")
	}
	s.AddTool(&mcp.Tool{Name: name, Description: description, InputSchema: input, OutputSchema: output,
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, IdempotentHint: true}}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		start := cfg.Clock()
		principal, ok := credential.FromContext(ctx)
		code := ""
		defer func() {
			if cfg.Logger != nil {
				cfg.Logger.Info("mcp tool", "account_id", principal.AccountID, "tool", name, "code", code, "latency", cfg.Clock().Sub(start))
			}
		}()
		fail := func(c, message string) (*mcp.CallToolResult, error) { code = c; return toolFailure(c, message), nil }
		if !ok || principal.AccountID <= 0 || principal.Scope != "mcp:read" {
			return fail("unavailable", "Service unavailable.")
		}
		if cfg.Limiter == nil {
			return fail("unavailable", "Service unavailable.")
		}
		class := limits.Read
		if name == "search" {
			class = limits.Search
		}
		if allowed, retry := cfg.Limiter.Allow(principal.AccountID, class); !allowed {
			seconds := max(int64(1), int64(math.Ceil(retry.Seconds())))
			return fail("rate_limited", fmt.Sprintf("Rate limited. Retry in %d seconds.", seconds))
		}
		raw := req.Params.Arguments
		var value any
		if len(raw) == 0 || json.Unmarshal(raw, &value) != nil || resolved.Validate(value) != nil {
			return fail("invalid_request", "invalid_request")
		}
		var in In
		if json.Unmarshal(raw, &in) != nil {
			return fail("invalid_request", "invalid_request")
		}
		out, err := run(ctx, in)
		if err != nil {
			c, message := errorMessage(err)
			return fail(c, message)
		}
		result, data, err := encodeResult(out, false)
		if err != nil || outputResolved.Validate(data) != nil {
			return fail("unavailable", "Service unavailable.")
		}
		return result, nil
	})
}

func schemaFor[T any]() *jsonschema.Schema {
	s, err := jsonschema.For[T](&jsonschema.ForOptions{TypeSchemas: map[reflect.Type]*jsonschema.Schema{
		reflect.TypeFor[retrieval.Date](): {Type: "string", Format: "date"},
	}})
	if err != nil {
		panic("invalid tool schema type")
	}
	return s
}
func ptrFloat(n float64) *float64 { return &n }
func ptrInt(n int) *int           { return &n }

// Empty and null fields are omitted in results, including nested recipe values.
func optionalEmptyFields(s *jsonschema.Schema) {
	var required []string
	for _, key := range s.Required {
		p := s.Properties[key]
		if p != nil && (p.Type == "boolean" || p.Type == "integer" || p.Type == "number") {
			required = append(required, key)
		}
	}
	s.Required = required
	for _, p := range s.Properties {
		optionalEmptyFields(p)
	}
	if s.Items != nil {
		optionalEmptyFields(s.Items)
	}
	if s.AdditionalProperties != nil {
		optionalEmptyFields(s.AdditionalProperties)
	}
	for _, p := range s.AnyOf {
		optionalEmptyFields(p)
	}
}
