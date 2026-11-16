package serve

import (
	"context"
	"slices"
)

func (e *Engine) validateMappings(r MappingsRequest) (MappingsRequest, error) {
	if !e.releaseMatches(r.Release) {
		return r, ErrReleaseExpired
	}
	if r.Limit < 0 || r.Limit > 200 || (r.Handle != "" && (r.From != "" || r.To != "")) {
		return r, ErrInvalidRequest
	}
	if r.Limit == 0 {
		r.Limit = 50
	}
	values := []string{r.Handle}
	if r.Handle == "" {
		values = []string{r.From, r.To}
	}
	for _, raw := range values {
		h, err := parseHandle(raw)
		if err != nil {
			return r, err
		}
		if (r.Handle != "") != (h.citation != "") {
			return r, ErrInvalidRequest
		}
		if _, found := e.versionFor(h); !found {
			return r, ErrInvalidRequest
		}
	}
	r.Relations = slices.Clone(r.Relations)
	for _, relation := range r.Relations {
		if !slices.Contains([]string{"equivalent", "subset", "superset", "intersects", "related"}, relation) {
			return r, ErrInvalidRequest
		}
	}
	slices.Sort(r.Relations)
	r.Relations = slices.Compact(r.Relations)
	if len(r.Relations) == 0 {
		r.Relations = nil
	}
	return r, nil
}

func mappingDigest(r MappingsRequest) string {
	return digestValue(struct {
		Handle, From, To string
		Relations        []string
		Limit            int
	}{r.Handle, r.From, r.To, r.Relations, r.Limit})
}

func (e *Engine) Mappings(ctx context.Context, request MappingsRequest, caller Caller) (MappingsResponse, error) {
	p := e.project(caller)
	response := MappingsResponse{Metadata: e.metadata(p, caller)}
	r, err := e.validateMappings(request)
	if err != nil {
		return response, err
	}
	c := continuation{Operation: "mappings", Release: e.release, Access: p.version, Digest: mappingDigest(r)}
	if r.Cursor != "" {
		c, err = e.decodeContinuation(r.Cursor, "mappings", c.Digest, p)
		if err != nil {
			return response, err
		}
		if len(c.Key) != 8 || c.Handle != "" || c.Mode != "" || c.Unit != 0 || c.Part != 0 {
			return response, ErrInvalidRequest
		}
	}
	ctx, tx, cancel, err := e.beginOperation(ctx)
	if err != nil {
		return response, err
	}
	defer cancel()
	defer func() { _ = tx.Rollback(context.Background()) }()
	rows, err := loadMappingRows(ctx, tx, r)
	if err != nil {
		return response, err
	}
	start := 0
	for start < len(rows) && len(c.Key) > 0 && slices.Compare(rows[start].key, c.Key) <= 0 {
		start++
	}
	page := rows[start:min(len(rows), start+r.Limit)]
	if err = mappingEvidence(ctx, tx, page, p); err != nil {
		return response, err
	}
	if tx.Commit(ctx) != nil {
		return response, ErrUnavailable
	}
	for _, row := range page {
		response.Edges = append(response.Edges, boundedEvidence(row.value))
	}
	if start+len(page) < len(rows) {
		c.Key = page[len(page)-1].key
		response.NextCursor = e.encodeContinuation(c)
	}
	return response, nil
}

func boundedEvidence(edge MappingEdge) MappingEdge {
	runes := []rune(edge.Evidence)
	if len(runes) > 2000 {
		edge.Evidence = string(runes[:cutBoundary(runes, 2000)])
		value := true
		edge.EvidenceTruncated = &value
	}
	return edge
}
