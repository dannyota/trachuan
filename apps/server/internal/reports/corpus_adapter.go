package reports

import (
	"context"
	"fmt"

	"danny.vn/trachuan/apps/server/internal/release"
	"danny.vn/trachuan/internal/retrieval"
)

// ReleaseResolver adapts the server's active release resolver to report leases.
type ReleaseResolver struct{ Resolver *release.Resolver }

// Resolve pins the active engine through report validation and admission.
func (r ReleaseResolver) Resolve(ctx context.Context, country, requested string) (Lease, error) {
	lease, err := r.Resolver.Resolve(ctx, country, requested)
	if err != nil {
		return Lease{}, fmt.Errorf("resolve report release: %w", err)
	}
	return Lease{Corpus: EngineCorpus{Engine: lease.Engine}, Release: lease.Release, Done: lease.Done}, nil
}

// EngineCorpus adapts immutable retrieval identity lookups to report validation.
type EngineCorpus struct{ Engine *retrieval.Engine }

// Document returns existence and the original corpus number for a document key.
func (c EngineCorpus) Document(ctx context.Context, key string) (Document, bool, error) {
	doc, found, err := c.Engine.LookupDocument(ctx, key)
	return Document{Number: doc.Number}, found, err
}

// AnchorExists checks the anchor inside the named document.
func (c EngineCorpus) AnchorExists(ctx context.Context, key, anchor string) (bool, error) {
	return c.Engine.AnchorExists(ctx, key, anchor)
}

// HasNumber tests exact normalized-number membership in this release.
func (c EngineCorpus) HasNumber(ctx context.Context, number string) (bool, error) {
	return c.Engine.HasNumber(ctx, number)
}

// HasNumberSegment tests a segment against the engine's cached release set.
func (c EngineCorpus) HasNumberSegment(ctx context.Context, segment string) (bool, error) {
	return c.Engine.HasNumberSegment(ctx, segment)
}
