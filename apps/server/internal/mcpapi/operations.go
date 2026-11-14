package mcpapi

import (
	"context"
	"encoding/json"
	"time"

	"danny.vn/trachuan/internal/retrieval"
)

// Operation validates, resolves and serializes a tool request. Authentication
// and per-account admission belong to the transport calling it.
type Operation func(context.Context, json.RawMessage) (json.RawMessage, error)

// Operations holds the same corpus operations used by the MCP tools.
type Operations struct {
	Search Operation
	Read   Operation
	Status Operation
	cfg    Config
}

// NewOperations constructs shared operations with the MCP schemas and caches.
func NewOperations(cfg Config) *Operations {
	cfg.OmitCompactRelated = false
	if cfg.Clock == nil {
		cfg.Clock = time.Now
	}
	return &Operations{Search: operation(cfg, "search", cfg.search), Read: operation(cfg, "read", cfg.read), Status: operation(cfg, "corpus_status", cfg.status), cfg: cfg}
}

func operation[In, Out any](cfg Config, name string, run func(context.Context, In) (Out, error)) Operation {
	_, _, call := toolCall(cfg, name, run)
	return func(ctx context.Context, raw json.RawMessage) (json.RawMessage, error) {
		ctx = context.WithValue(ctx, requestInstantKey{}, cfg.Clock())
		result, err := call(ctx, raw)
		if err != nil {
			return nil, err
		}
		return result.StructuredContent.(json.RawMessage), nil
	}
}

// Document reads one whole-document page with the website's byte budgets.
func (o *Operations) Document(ctx context.Context, key, release, next string) (json.RawMessage, error) {
	item := retrieval.ReadItem{Handle: key}
	if next != "" {
		item = retrieval.ReadItem{Next: next}
	}
	result, err := o.cfg.readRequest(ctx, "vn", retrieval.ReadRequest{Release: release, Items: []retrieval.ReadItem{item}, PageCap: 2_000_000, PartSize: 1_000_000, WholeDocument: true, DocumentKey: key})
	if err != nil {
		return nil, err
	}
	if len(result.Items) != 1 {
		return nil, retrieval.ErrUnavailable
	}
	readItem := result.Items[0]
	page := struct {
		Release     string                       `json:"release"`
		FetchSHA256 string                       `json:"fetch_sha256,omitempty"`
		Item        retrieval.ItemResult         `json:"item"`
		Nodes       map[string]retrieval.NodeRef `json:"nodes"`
	}{Release: result.Release, Item: readItem, Nodes: result.Nodes}
	if readItem.Error == "" {
		page.FetchSHA256 = readItem.FetchSHA256
	}
	encoded, _, err := encodeResult(page, false)
	if err != nil {
		return nil, err
	}
	return encoded.StructuredContent.(json.RawMessage), nil
}
