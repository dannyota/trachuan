package mcpapi

import (
	"context"

	"danny.vn/trachuan/internal/frameworks/serve"
	"danny.vn/trachuan/internal/retrieval"
)

type readInput struct {
	Country string               `json:"country"`
	Release string               `json:"release,omitempty"`
	Items   []retrieval.ReadItem `json:"items"`
}

func (cfg Config) read(ctx context.Context, in readInput) (retrieval.ReadResult, error) {
	handles := make([]string, len(in.Items))
	for i, item := range in.Items {
		handles[i] = item.Handle
	}
	if mixedReadHandles(handles) {
		return retrieval.ReadResult{}, invalidRequest{}
	}
	return cfg.readRequest(ctx, in.Country, retrieval.ReadRequest{Release: in.Release, Items: in.Items})
}

func mixedReadHandles(handles []string) bool {
	law, framework := false, false
	for _, handle := range handles {
		_, _, err := retrieval.ParseHandle(handle)
		law = law || err == nil
		framework = framework || serve.IsHandle(handle)
	}
	return law && framework
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
		ctx = retrieval.WithCaches(ctx, scope.Documents, scope.Sequences, nil)
	}
	req.Release = lease.Release
	return lease.Engine.Read(ctx, req)
}
