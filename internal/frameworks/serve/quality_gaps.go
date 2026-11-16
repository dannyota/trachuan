package serve

import (
	"context"
	"slices"
)

type GapsRequest struct {
	Release, Kind, Cursor string
	Versions              []string
}

type GapsResponse struct {
	Metadata
	RetrievalRecipeStatus string `json:"retrieval_recipe_status"`
	Gaps                  []Gap  `json:"gaps,omitempty"`
	NextCursor            string `json:"next_cursor,omitempty"`
}

const qualityGapsSQL = `SELECT g.kind,'fw:'||v.framework_code||':'||v.version,g.subject,g.detail
 FROM quality_gap g JOIN framework_version v ON v.id=g.version_id
 WHERE g.version_id=ANY($1::bigint[]) AND (g.class='public' OR (g.class='text' AND g.version_id=ANY($2::bigint[])))
 AND ($3::text='' OR g.kind=$3)
 AND (NOT $4::boolean OR (g.kind,('fw:'||v.framework_code||':'||v.version) COLLATE "C",g.subject,g.detail)>($5,$6,$7,$8))
 ORDER BY g.kind COLLATE "C",('fw:'||v.framework_code||':'||v.version) COLLATE "C",g.subject COLLATE "C",g.detail COLLATE "C" LIMIT 201`

func (e *Engine) validateGaps(r GapsRequest) (GapsRequest, []int64, error) {
	if !e.releaseMatches(r.Release) {
		return r, nil, ErrReleaseExpired
	}
	kinds := []string{"mapping_unresolved", "mapping_dropped", "title_missing", "ocr_blocked", "ocr_flagged", "text_incomplete", "chunk_omission", "embedding_input_too_long", "embedding_input_bytes_exceeded"}
	if r.Kind != "" && !slices.Contains(kinds, r.Kind) {
		return r, nil, ErrInvalidRequest
	}
	r.Versions = slices.Clone(r.Versions)
	slices.Sort(r.Versions)
	r.Versions = slices.Compact(r.Versions)
	if len(r.Versions) == 0 {
		r.Versions = nil
	}
	for _, raw := range r.Versions {
		h, err := parseHandle(raw)
		if err != nil || h.citation != "" {
			return r, nil, ErrInvalidRequest
		}
		if _, found := e.versionFor(h); !found {
			return r, nil, ErrInvalidRequest
		}
	}
	ids := []int64{}
	for _, v := range e.versions {
		if len(r.Versions) == 0 || slices.Contains(r.Versions, v.handle) {
			ids = append(ids, v.id)
		}
	}
	return r, ids, nil
}

func (e *Engine) QualityGaps(ctx context.Context, request GapsRequest, caller Caller) (GapsResponse, error) {
	p := e.project(caller)
	response := GapsResponse{Metadata: e.metadata(p, caller), RetrievalRecipeStatus: e.recipe.Status}
	r, ids, err := e.validateGaps(request)
	if err != nil {
		return response, err
	}
	digest := digestValue(struct {
		Versions []string
		Kind     string
	}{r.Versions, r.Kind})
	c := continuation{Operation: "quality_gaps", Release: e.release, Access: p.version, Digest: digest}
	if r.Cursor != "" {
		c, err = e.decodeContinuation(r.Cursor, "quality_gaps", digest, p)
		if err != nil {
			return response, err
		}
		if len(c.Key) != 4 || c.Handle != "" || c.Mode != "" || c.Unit != 0 || c.Part != 0 {
			return response, ErrInvalidRequest
		}
	}
	ctx, tx, cancel, err := e.beginOperation(ctx)
	if err != nil {
		return response, err
	}
	defer cancel()
	defer func() { _ = tx.Rollback(context.Background()) }()
	key := make([]string, 4)
	copy(key, c.Key)
	rows, err := tx.Query(ctx, qualityGapsSQL, ids, p.readable, r.Kind, len(c.Key) > 0, key[0], key[1], key[2], key[3])
	if err != nil {
		return response, ErrUnavailable
	}
	var gaps []Gap
	for rows.Next() {
		var gap Gap
		if rows.Scan(&gap.Kind, &gap.Version, &gap.Subject, &gap.Detail) != nil {
			rows.Close()
			return response, ErrUnavailable
		}
		gaps = append(gaps, gap)
	}
	err = rows.Err()
	rows.Close()
	if err != nil || tx.Commit(ctx) != nil {
		return response, ErrUnavailable
	}
	if len(gaps) > 200 {
		gaps = gaps[:200]
		last := gaps[len(gaps)-1]
		c.Key = []string{last.Kind, last.Version, last.Subject, last.Detail}
		response.NextCursor = e.encodeContinuation(c)
	}
	response.Gaps = gaps
	return response, nil
}
