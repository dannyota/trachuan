package mcpapi

import (
	"context"
	"encoding/json"
	"time"

	"danny.vn/trachuan/internal/retrieval"
)

type Operation func(context.Context, json.RawMessage) (json.RawMessage, error)

type Operations struct {
	Search Operation
	Read   Operation
	Status Operation
	cfg    Config
}

func NewOperations(cfg Config) *Operations {
	cfg.Frameworks = false
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
	encoded, _, err := encodeResult(readItem, false)
	if err != nil {
		return nil, err
	}
	page := struct {
		Files       *[]retrieval.SourceFile      `json:"files,omitempty"`
		Release     string                       `json:"release"`
		FetchSHA256 string                       `json:"fetch_sha256,omitempty"`
		Item        json.RawMessage              `json:"item"`
		Nodes       map[string]retrieval.NodeRef `json:"nodes,omitempty"`
	}{Release: result.Release, Item: encoded.StructuredContent.(json.RawMessage), Nodes: result.Nodes}
	if readItem.Error == "" {
		page.FetchSHA256 = readItem.FetchSHA256
		files := readItem.Files
		if files == nil {
			files = []retrieval.SourceFile{}
		}
		page.Files = &files
	}
	return json.Marshal(page)
}
