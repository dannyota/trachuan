package mcpapi

import (
	"context"
	"encoding/json"

	"danny.vn/trachuan/apps/server/internal/credential"
	"danny.vn/trachuan/internal/frameworks/serve"
)

type FrameworkEngine interface {
	FrameworkCodes() []string
	Search(context.Context, serve.Request, serve.Caller) (serve.Response, error)
	Read(context.Context, serve.ReadRequest, serve.Caller) (serve.ReadResponse, error)
	CorpusStatus(context.Context, serve.StatusRequest, serve.Caller) (serve.StatusResponse, error)
	QualityGaps(context.Context, serve.GapsRequest, serve.Caller) (serve.GapsResponse, error)
	Mappings(context.Context, serve.MappingsRequest, serve.Caller) (serve.MappingsResponse, error)
}

type FrameworkLease struct {
	Engine FrameworkEngine
	Done   func()
}

type FrameworkResolver interface {
	ResolveFrameworks(context.Context, string) (FrameworkLease, error)
}

type frameworkArguments struct {
	frameworkSearchInput
	Items    []serve.ReadItem `json:"items"`
	Kind     string           `json:"kind"`
	Handle   string           `json:"handle"`
	From     string           `json:"from"`
	To       string           `json:"to"`
	Relation []string         `json:"relation"`
}

func (cfg Config) frameworkCall(ctx context.Context, name string, raw json.RawMessage) (any, error) {
	var in frameworkArguments
	if json.Unmarshal(raw, &in) != nil {
		return nil, serve.ErrInvalidRequest
	}
	if name == "read" {
		handles := make([]string, len(in.Items))
		for i, item := range in.Items {
			handles[i] = item.Handle
		}
		if mixedReadHandles(handles) {
			return nil, serve.ErrInvalidRequest
		}
	}
	principal, ok := credential.FromContext(ctx)
	if !ok || principal.AccountID <= 0 || cfg.FrameworkResolver == nil || cfg.FrameworkAccess == nil {
		return nil, serve.ErrUnavailable
	}
	lease, err := cfg.FrameworkResolver.ResolveFrameworks(ctx, in.Release)
	if lease.Done != nil {
		defer lease.Done()
	}
	if err != nil {
		return nil, err
	}
	if lease.Engine == nil || lease.Done == nil {
		return nil, serve.ErrUnavailable
	}
	caller, err := cfg.FrameworkAccess(ctx, principal.AccountID, lease.Engine.FrameworkCodes())
	if err != nil {
		return nil, serve.ErrUnavailable
	}
	switch name {
	case "search":
		return lease.Engine.Search(ctx, serve.Request{Query: in.Query, Release: in.Release, Limit: in.Limit, Cursor: in.Cursor, Filters: map[string][]string{"frameworks": in.Frameworks, "versions": in.Versions}}, caller)
	case "read":
		return lease.Engine.Read(ctx, serve.ReadRequest{Release: in.Release, Items: in.Items}, caller)
	case "corpus_status":
		return lease.Engine.CorpusStatus(ctx, serve.StatusRequest{Release: in.Release}, caller)
	case "quality_gaps":
		return lease.Engine.QualityGaps(ctx, serve.GapsRequest{Release: in.Release, Versions: in.Versions, Kind: in.Kind, Cursor: in.Cursor}, caller)
	case "mappings":
		return lease.Engine.Mappings(ctx, serve.MappingsRequest{Release: in.Release, Handle: in.Handle, From: in.From, To: in.To, Relations: in.Relation, Limit: in.Limit, Cursor: in.Cursor}, caller)
	default:
		return nil, serve.ErrInvalidRequest
	}
}
