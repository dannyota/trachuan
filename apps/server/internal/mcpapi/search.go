package mcpapi

import (
	"context"
	"time"

	"danny.vn/trachuan/internal/retrieval"
)

type searchInput struct {
	Country     string           `json:"country"`
	Query       string           `json:"query"`
	AsOf        string           `json:"as_of,omitempty"`
	InForceOnly *bool            `json:"in_force_only,omitempty"`
	Detail      retrieval.Detail `json:"detail,omitempty"`
	Limit       *int             `json:"limit,omitempty"`
	Release     string           `json:"release,omitempty"`
}

func (cfg Config) search(ctx context.Context, in searchInput) (retrieval.SearchResult, error) {
	req, detail, validation, err := prepareSearch(in)
	if err != nil {
		return retrieval.SearchResult{}, err
	}
	engine, rel, done, err := cfg.resolve(ctx, in.Country, in.Release)
	if err != nil {
		return retrieval.SearchResult{}, err
	}
	defer done()
	if cfg.Caches != nil {
		scope := cfg.Caches.Scope(rel)
		ctx = retrieval.WithCaches(ctx, scope.Documents, scope.Sequences)
	}
	req.Instant = cfg.requestInstant(ctx)
	return searchResolved(ctx, engine, rel, req, detail, validation)
}

func prepareSearch(in searchInput) (retrieval.Request, retrieval.Detail, retrieval.Validation, error) {
	detail := in.Detail
	if detail == "" {
		detail = retrieval.DetailCompact
	}
	req := retrieval.Request{Query: in.Query, Limit: retrieval.DefaultLimit(detail), InForceOnly: in.InForceOnly}
	if in.Limit != nil {
		req.Limit = *in.Limit
	}
	if in.AsOf != "" {
		date, err := time.Parse(time.DateOnly, in.AsOf)
		if err != nil || date.Year() < 1 {
			return retrieval.Request{}, "", retrieval.Validation{}, invalidRequest{}
		}
		req.AsOf = &retrieval.Date{Year: date.Year(), Month: date.Month(), Day: date.Day()}
	}
	validation, err := retrieval.ValidateQuery(in.Query)
	if err != nil {
		return retrieval.Request{}, "", retrieval.Validation{}, invalidRequest{}
	}
	return req, detail, validation, nil
}

func searchResolved(ctx context.Context, engine Engine, rel string, req retrieval.Request, detail retrieval.Detail, validation retrieval.Validation) (retrieval.SearchResult, error) {
	if !validation.Accepted {
		return retrieval.SearchResult{Release: rel, Abstain: true, Reason: validation.Reason, Message: validation.Message}, nil
	}
	response, err := engine.Search(ctx, req)
	if err != nil {
		return retrieval.SearchResult{}, err
	}
	return engine.Present(ctx, response, detail)
}
func (cfg Config) resolve(ctx context.Context, country, rel string) (Engine, string, func(), error) {
	if cfg.Resolver == nil {
		return nil, "", nil, retrieval.ErrUnavailable
	}
	engine, active, done, err := cfg.Resolver.Resolve(ctx, country, rel)
	if err != nil {
		if done != nil {
			done()
		}
		return nil, "", nil, err
	}
	if engine == nil || done == nil || active == "" {
		if done != nil {
			done()
		}
		return nil, "", nil, retrieval.ErrUnavailable
	}
	return engine, active, done, nil
}

type requestInstantKey struct{}

func (cfg Config) requestInstant(ctx context.Context) time.Time {
	if instant, ok := ctx.Value(requestInstantKey{}).(time.Time); ok {
		return instant
	}
	return cfg.Clock()
}
