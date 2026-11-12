package mcpapi

import (
	"context"
	"encoding/json"

	"danny.vn/trachuan/internal/retrieval"
	"github.com/google/jsonschema-go/jsonschema"
)

type taxonomyEngine interface {
	Taxonomy(context.Context, retrieval.TaxonomyRequest) (retrieval.TaxonomyResult, error)
	TaxonomyNode(context.Context, string) (retrieval.NodePage, error)
}

type taxonomyInput struct {
	Country string `json:"country"`
	Node    string `json:"node,omitempty"`
	Depth   *int   `json:"depth,omitempty"`
	Q       string `json:"q,omitempty"`
}

func (cfg Config) taxonomy(ctx context.Context, in taxonomyInput) (retrieval.TaxonomyResult, error) {
	lease, err := cfg.resolve(ctx, in.Country, "")
	if err != nil {
		return retrieval.TaxonomyResult{}, err
	}
	defer lease.Done()
	engine, ok := lease.Engine.(taxonomyEngine)
	if !ok {
		return retrieval.TaxonomyResult{}, retrieval.ErrUnavailable
	}
	req := retrieval.TaxonomyRequest{Node: in.Node, Q: in.Q}
	if in.Depth != nil {
		req.Depth = *in.Depth
	}
	return engine.Taxonomy(ctx, req)
}

// Taxonomy returns the public taxonomy tree through the shared tool contract.
func (o *Operations) Taxonomy(ctx context.Context, raw json.RawMessage) (json.RawMessage, error) {
	return operation(o.cfg, "taxonomy", o.cfg.taxonomy)(ctx, raw)
}

// TaxonomyNode returns a public node page from the active release.
func (o *Operations) TaxonomyNode(ctx context.Context, code string) (json.RawMessage, error) {
	lease, err := o.cfg.resolve(ctx, "vn", "")
	if err != nil {
		return nil, err
	}
	defer lease.Done()
	engine, ok := lease.Engine.(taxonomyEngine)
	if !ok {
		return nil, retrieval.ErrUnavailable
	}
	page, err := engine.TaxonomyNode(ctx, code)
	if err != nil {
		return nil, err
	}
	result, _, err := encodeResult(page, false)
	if err != nil {
		return nil, err
	}
	return result.StructuredContent.(json.RawMessage), nil
}

func taxonomySchema() *jsonschema.Schema {
	node := &jsonschema.Schema{Type: "object", Properties: map[string]*jsonschema.Schema{
		"code": {Type: "string"}, "name": {Type: "string"}, "description": {Type: "string"}, "level": {Type: "integer"}, "origin": {Type: "string"}, "provisions": {Type: "integer"}, "documents": {Type: "integer"}, "children": {Type: "array", Items: &jsonschema.Schema{Ref: "#/anyOf/0/$defs/node"}},
	}}
	return &jsonschema.Schema{Type: "object", Defs: map[string]*jsonschema.Schema{"node": node}, Properties: map[string]*jsonschema.Schema{
		"release": {Type: "string"}, "nodes": {Type: "array", Items: &jsonschema.Schema{Ref: "#/anyOf/0/$defs/node"}}, "truncated": {Type: "boolean"},
	}, Required: []string{"truncated"}}
}
