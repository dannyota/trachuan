package mcpapi

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"slices"
	"strings"
	"sync"
	"time"

	"golang.org/x/text/unicode/norm"

	"danny.vn/trachuan/apps/server/internal/cache"
	"danny.vn/trachuan/internal/retrieval"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Caches struct {
	mu         sync.Mutex
	Search     *cache.Scoped[[]byte]
	Documents  *cache.Scoped[[]byte]
	Sequences  *cache.Scoped[[]byte]
	TopicNodes *cache.Scoped[[]byte]
}

func NewCaches(clock func() time.Time) *Caches {
	size := func(v []byte) int { return len(v) }
	return &Caches{
		Search:     cache.NewScoped(cache.New("search_responses", 1_024_000_000, time.Hour, clock, size)),
		Documents:  cache.NewScoped(cache.New("document_entries", 64_000_000, 0, clock, size)),
		Sequences:  cache.NewScoped(cache.New("read_items", 512_000_000, 24*time.Hour, clock, size)),
		TopicNodes: cache.NewScoped(cache.New("topic_node_pages", 8_000_000, 0, clock, size)),
	}
}

func (c *Caches) ActivateRelease(release string, generation uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Search.ActivateRelease(release, generation)
	c.Documents.ActivateRelease(release, generation)
	c.Sequences.ActivateRelease(release, generation)
	c.TopicNodes.ActivateRelease(release, generation)
}

type RequestCaches struct {
	Search     cache.Handle[[]byte]
	Documents  cache.Handle[[]byte]
	Sequences  cache.Handle[[]byte]
	TopicNodes cache.Handle[[]byte]
}

func (c *Caches) Scope(release string, generation uint64) RequestCaches {
	c.mu.Lock()
	defer c.mu.Unlock()
	return RequestCaches{Search: c.Search.Scope(release, generation), Documents: c.Documents.Scope(release, generation), Sequences: c.Sequences.Scope(release, generation), TopicNodes: c.TopicNodes.Scope(release, generation)}
}

func (cfg Config) cachedSearch(ctx context.Context, in searchInput, validate func(any) error) (*mcp.CallToolResult, any, error) {
	req, detail, validation, err := prepareSearch(in)
	if err != nil {
		return nil, nil, err
	}
	lease, err := cfg.resolve(ctx, in.Country, in.Release)
	if err != nil {
		return nil, nil, err
	}
	defer lease.Done()
	rel := lease.Release
	scope := cfg.Caches.Scope(rel, lease.Generation)
	ctx = retrieval.WithCaches(ctx, scope.Documents, scope.Sequences)
	req.Instant = cfg.requestInstant(ctx)
	now := req.Instant.In(time.FixedZone("Asia/Ho_Chi_Minh", 7*60*60))
	force := true
	if in.InForceOnly != nil {
		force = *in.InForceOnly
	}
	canonical := map[string]any{"country": in.Country, "query": in.Query, "as_of": in.AsOf, "in_force_only": force, "detail": detail, "limit": req.Limit, "release": rel, "nodes": canonicalFilter(in.Nodes), "doc_types": canonicalFilter(in.DocTypes), "issuers": canonicalFilter(in.Issuers), "documents": canonicalFilter(in.Documents), "document_keys": exactDocumentFilter(in.Documents), "cursor": in.Cursor}
	raw, _ := json.Marshal([]any{rel, now.Format(time.DateOnly), canonical})
	key := sha256.Sum256(raw)
	if raw, ok := scope.Search.Get(rel, key); ok {
		var data any
		if err = json.Unmarshal(raw, &data); err != nil {
			return nil, nil, err
		}
		return serializedResult(raw), data, nil
	}
	out, err := searchResolved(ctx, lease.Engine, rel, req, detail, validation)
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
		scope.Search.Put(rel, key, result.StructuredContent.(json.RawMessage))
	}
	return result, data, nil
}

func serializedResult(raw []byte) *mcp.CallToolResult {
	return &mcp.CallToolResult{StructuredContent: json.RawMessage(raw), Content: []mcp.Content{&mcp.TextContent{Text: string(raw)}}}
}

func canonicalFilter(values []string) []string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		out = append(out, norm.NFC.String(strings.TrimSpace(v)))
	}
	slices.Sort(out)
	return slices.Compact(out)
}

func exactDocumentFilter(values []string) []string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		out = append(out, strings.TrimSpace(v))
	}
	slices.Sort(out)
	return slices.Compact(out)
}
