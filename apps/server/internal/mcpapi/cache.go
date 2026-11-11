package mcpapi

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"time"

	"danny.vn/trachuan/apps/server/internal/cache"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Caches owns the release-scoped caches shared by MCP and retrieval engines.
// Query vectors are separate because they survive release changes.
type Caches struct {
	Search    *cache.Scoped[[]byte]
	Documents *cache.Scoped[[]byte]
	Sequences *cache.Scoped[[]byte]
}

// NewCaches creates caches with the host budgets. They remain disabled until
// the resolver adapter calls ActivateRelease before lending its first lease.
func NewCaches(clock func() time.Time) *Caches {
	size := func(v []byte) int { return len(v) }
	return &Caches{
		Search:    cache.NewScoped(cache.New("search_responses", 1_024_000_000, time.Hour, clock, size)),
		Documents: cache.NewScoped(cache.New("document_entries", 64_000_000, 0, clock, size)),
		Sequences: cache.NewScoped(cache.New("read_items", 512_000_000, 24*time.Hour, clock, size)),
	}
}

// ActivateRelease purges every corpus cache on a switch. The resolver adapter
// must serialize switch notifications and call this before returning a lease.
// Pass Documents and Sequences to retrieval.Options when opening engines.
func (c *Caches) ActivateRelease(release string) {
	c.Search.ActivateRelease(release)
	c.Documents.ActivateRelease(release)
	c.Sequences.ActivateRelease(release)
}

func (cfg Config) cachedSearch(ctx context.Context, in searchInput, validate func(any) error) (*mcp.CallToolResult, any, error) {
	req, detail, validation, err := prepareSearch(in)
	if err != nil {
		return nil, nil, err
	}
	engine, rel, done, err := cfg.resolve(ctx, in.Country, in.Release)
	if err != nil {
		return nil, nil, err
	}
	defer done()
	now := cfg.Clock().In(time.FixedZone("Asia/Ho_Chi_Minh", 7*60*60))
	force := true
	if in.InForceOnly != nil {
		force = *in.InForceOnly
	}
	canonical := map[string]any{"country": in.Country, "query": in.Query, "as_of": in.AsOf, "in_force_only": force, "detail": detail, "limit": req.Limit, "release": rel}
	raw, _ := json.Marshal([]any{rel, now.Format(time.DateOnly), canonical})
	key := sha256.Sum256(raw)
	if raw, ok := cfg.Caches.Search.Get(rel, key); ok {
		var data any
		if err = json.Unmarshal(raw, &data); err != nil {
			return nil, nil, err
		}
		return serializedResult(raw), data, nil
	}
	out, err := searchResolved(ctx, engine, rel, req, detail, validation)
	if err != nil {
		return nil, nil, err
	}
	result, data, err := encodeResult(out, false)
	if err != nil {
		return nil, nil, err
	}
	if err = validate(data); err != nil {
		return nil, nil, err
	}
	if ctx.Err() == nil {
		cfg.Caches.Search.Put(rel, key, result.StructuredContent.(json.RawMessage))
	}
	return result, data, nil
}

func serializedResult(raw []byte) *mcp.CallToolResult {
	return &mcp.CallToolResult{StructuredContent: json.RawMessage(raw), Content: []mcp.Content{&mcp.TextContent{Text: string(raw)}}}
}
