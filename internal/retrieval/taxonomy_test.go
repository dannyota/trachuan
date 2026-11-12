package retrieval

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestTaxonomy_Errors(t *testing.T) {
	e := &Engine{release: Release{SchemaVersion: 4}}
	for _, tt := range []struct {
		name string
		req  TaxonomyRequest
		want error
	}{
		{"depth", TaxonomyRequest{Depth: 4}, ErrInvalidRequest},
		{"code", TaxonomyRequest{Node: "bad"}, ErrInvalidRequest},
		{"query", TaxonomyRequest{Q: strings.Repeat("a", 101)}, ErrInvalidRequest},
		{"unserved", TaxonomyRequest{Node: "1"}, ErrNotFound},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := e.Taxonomy(context.Background(), tt.req)
			if !errors.Is(err, tt.want) {
				t.Fatalf("error = %v, want %v", err, tt.want)
			}
		})
	}
	for _, tt := range []struct {
		code string
		want error
	}{{"bad", ErrInvalidRequest}, {"1", ErrNotFound}} {
		_, err := e.TaxonomyNode(context.Background(), tt.code)
		if !errors.Is(err, tt.want) {
			t.Fatalf("node error = %v, want %v", err, tt.want)
		}
	}
}
