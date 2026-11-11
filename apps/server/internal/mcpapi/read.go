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
	if len(in.Items) > retrieval.ReadItemsMax {
		return retrieval.ReadResult{}, invalidRequest{}
	}
	lease, err := cfg.resolve(ctx, in.Country, in.Release)
	if err != nil {
		return retrieval.ReadResult{}, err
	}
	defer lease.Done()
	if cfg.Caches != nil {
		scope := cfg.Caches.Scope(lease.Release, lease.Generation)
		ctx = retrieval.WithCaches(ctx, scope.Documents, scope.Sequences)
	}
	return lease.Engine.Read(ctx, retrieval.ReadRequest{Release: lease.Release, Items: in.Items})
}
