package mcpapi

import (
	"context"

	"danny.vn/trachuan/internal/retrieval"
)

type readInput struct {
	Country string               `json:"country"`
	Release string               `json:"release,omitempty"`
	Items   []retrieval.ReadItem `json:"items"`
}

func (cfg Config) read(ctx context.Context, in readInput) (retrieval.ReadResult, error) {
	return cfg.readRequest(ctx, in.Country, retrieval.ReadRequest{Release: in.Release, Items: in.Items})
}

func (cfg Config) readRequest(ctx context.Context, country string, req retrieval.ReadRequest) (retrieval.ReadResult, error) {
	if len(req.Items) > retrieval.ReadItemsMax {
		return retrieval.ReadResult{}, invalidRequest{}
	}
	lease, err := cfg.resolve(ctx, country, req.Release)
	if err != nil {
		return retrieval.ReadResult{}, err
	}
	defer lease.Done()
	if cfg.Caches != nil {
		scope := cfg.Caches.Scope(lease.Release, lease.Generation)
		ctx = retrieval.WithCaches(ctx, scope.Documents, scope.Sequences)
	}
	req.Release = lease.Release
	return lease.Engine.Read(ctx, req)
}
