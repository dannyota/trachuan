package mcpapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"math"
	"net/http"
	"reflect"
	"time"

	"danny.vn/trachuan/apps/server/internal/apierror"
	"danny.vn/trachuan/apps/server/internal/credential"
	"danny.vn/trachuan/apps/server/internal/limits"
	"danny.vn/trachuan/apps/server/internal/reports"
	"danny.vn/trachuan/internal/frameworks/serve"
	"danny.vn/trachuan/internal/retrieval"
	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Engine interface {
	Search(context.Context, retrieval.Request) (retrieval.Response, error)
	Present(context.Context, retrieval.Response, retrieval.Detail) (retrieval.SearchResult, error)
	Read(context.Context, retrieval.ReadRequest) (retrieval.ReadResult, error)
	Status(context.Context) (retrieval.CorpusStatus, error)
	Gaps(context.Context, string, string, int64, int) ([]retrieval.Gap, int64, error)
}

type Lease struct {
	Engine     Engine
	Release    string
	Generation uint64
	Done       func()
}

type Resolver interface {
	Resolve(context.Context, string, string) (Lease, error)
}

type Limiter interface {
	Allow(int64, limits.Class) (bool, time.Duration)
}

type Reports interface {
	Submit(context.Context, int64, reports.Input) (reports.Result, error)
}

type Config struct {
	Frameworks         bool
	FrameworkResolver  FrameworkResolver
	FrameworkAccess    func(context.Context, int64, []string) (serve.Caller, error)
	OmitCompactRelated bool
	Resolver           Resolver
	Limiter            Limiter
	Logger             *slog.Logger
	Clock              func() time.Time
	Caches             *Caches
	Reports            Reports
}

func New(cfg Config) http.Handler {
	if cfg.Clock == nil {
		cfg.Clock = time.Now
	}
	s := mcp.NewServer(&mcp.Implementation{Name: "trachuan", Title: "Tra Chuẩn", Version: "1"}, &mcp.ServerOptions{
		Capabilities: &mcp.ServerCapabilities{Tools: &mcp.ToolCapabilities{}},
	})
	registerTools(s, cfg)
	transport := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return s }, &mcp.StreamableHTTPOptions{
		Stateless: true, JSONResponse: true, MaxRequestBodyBytes: 64 << 10, PropagateRequestCancellation: true, DisableLocalhostProtection: true,
	})
	return noStore(guard(transport))
}

type countryInput struct {
	Country string `json:"country"`
}

func registerTools(s *mcp.Server, cfg Config) {
	register(s, cfg, "guide", "How to search, read and cite Vietnamese law.", func(_ context.Context, _ countryInput) (guideResult, error) {
		text := guideText
		if cfg.Frameworks {
			text += "\n" + frameworkGuideText
		}
		return guideResult{Guide: text}, nil
	})
	register(s, cfg, "taxonomy", "Find served taxonomy node codes.", cfg.taxonomy)
	register(s, cfg, "corpus_status", "Counts and coverage of the active corpus.", cfg.status)
	register(s, cfg, "quality_gaps", "Page through recorded corpus quality gaps; use node to filter by topic.", cfg.gaps)
	register(s, cfg, "search", "Search Vietnamese law; compact hits by default.", cfg.search)
	register(s, cfg, "read", "Read chosen handles in one batch; follow every next token.", cfg.read)
	if cfg.Frameworks {
		input, output, call := frameworkToolCall(cfg, "mappings", nil, nil, nil)
		registerHandler(s, cfg, "mappings", "Read mapping edges at a framework control or between framework versions.", input, output, call, &mcp.ToolAnnotations{ReadOnlyHint: true, IdempotentHint: true})
	}
	no := false
	registerAnnotated(s, cfg, "report_issue", "Report a corpus defect using structured fields only.", cfg.reportIssue,
		&mcp.ToolAnnotations{ReadOnlyHint: false, DestructiveHint: &no, IdempotentHint: true, OpenWorldHint: &no})
}

func register[In, Out any](s *mcp.Server, cfg Config, name, description string, run func(context.Context, In) (Out, error)) {
	registerAnnotated(s, cfg, name, description, run, &mcp.ToolAnnotations{ReadOnlyHint: true, IdempotentHint: true})
}

func registerAnnotated[In, Out any](s *mcp.Server, cfg Config, name, description string, run func(context.Context, In) (Out, error), annotations *mcp.ToolAnnotations) {
	input, output, call := toolCall(cfg, name, run)
	registerHandler(s, cfg, name, description, input, output, call, annotations)
}

func registerHandler(s *mcp.Server, cfg Config, name, description string, input, output *jsonschema.Schema, call func(context.Context, json.RawMessage) (*mcp.CallToolResult, error), annotations *mcp.ToolAnnotations) {
	s.AddTool(&mcp.Tool{Name: name, Description: description, InputSchema: input, OutputSchema: output,
		Annotations: annotations}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		start := cfg.Clock()
		latencyStart := time.Now()
		ctx = context.WithValue(ctx, requestInstantKey{}, start)
		principal, ok := credential.FromContext(ctx)
		code := ""
		defer func() {
			if cfg.Logger != nil {
				cfg.Logger.Info("mcp tool", "account_id", principal.AccountID, "tool", name, "code", code, "latency", time.Since(latencyStart))
			}
		}()
		fail := func(c, message string) (*mcp.CallToolResult, error) { code = c; return toolFailure(c, message), nil }
		if !ok || principal.AccountID <= 0 || principal.Scope != "mcp:read" {
			return fail("unavailable", apierror.Message("unavailable"))
		}
		if cfg.Limiter == nil {
			return fail("unavailable", apierror.Message("unavailable"))
		}
		class := limits.Read
		if name == "search" {
			class = limits.Search
		}
		if allowed, retry := cfg.Limiter.Allow(principal.AccountID, class); !allowed {
			seconds := max(int64(1), int64(math.Ceil(retry.Seconds())))
			return fail("rate_limited", apierror.RetryMessage(int(seconds)))
		}
		result, err := call(ctx, req.Params.Arguments)
		if err != nil {
			c, message := errorMessage(err)
			return fail(c, message)
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
