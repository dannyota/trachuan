package serve

import (
	"context"

	"danny.vn/trachuan/internal/retrieval"
	"github.com/jackc/pgx/v5"
)

func (e *Engine) hydrate(ctx context.Context, tx pgx.Tx, page []ranked, controls map[int64]*control, p projection, lookup bool) ([]Hit, map[string]Framework, error) {
	versions := map[int64]version{}
	for _, v := range e.versions {
		versions[v.id] = v
	}
	frameworks := map[string]Framework{}
	hits := make([]Hit, 0, len(page))
	readableIDs, chunkIDs := []int64{}, []int64{}
	for _, r := range page {
		c := controls[r.controlID]
		v := versions[c.versionID]
		info := v.info
		info.Readable = p.allowed[v.id]
		frameworks[v.handle] = info
		hit := Hit{Handle: v.handle + "#" + encodeCitation(c.citation), Citation: c.citation, Kind: c.kind, Title: c.title, Status: c.status, Path: c.path, Mappings: c.mappings, TextWithheld: !info.Readable}
		if lookup {
			value := true
			hit.Lookup = &value
		} else {
			value := r.score
			hit.Score = &value
		}
		if info.Readable {
			readableIDs = append(readableIDs, c.id)
			chunkIDs = append(chunkIDs, r.chunkID)
		}
		hits = append(hits, hit)
	}
	if len(readableIDs) == 0 {
		return hits, frameworks, nil
	}
	texts, err := hydrateText(ctx, tx, p.readable, readableIDs, chunkIDs, lookup)
	if err != nil {
		return nil, nil, err
	}
	for i, r := range page {
		c := controls[r.controlID]
		if !p.allowed[c.versionID] {
			continue
		}
		text, ok := texts[c.id]
		if !ok {
			return nil, nil, ErrUnavailable
		}
		hits[i].TitleOriginal = text.title
		hits[i].Snippet, _ = retrieval.Snippet(retrieval.Hit{Text: text.body, Complete: true}, retrieval.SnippetCap)
	}
	return hits, frameworks, nil
}

type controlText struct{ title, body string }

func hydrateText(ctx context.Context, tx pgx.Tx, readable, controls, chunks []int64, lookup bool) (map[int64]controlText, error) {
	sql := `SELECT t.control_id,t.title_original,b.content FROM control_text t
 JOIN control c ON c.id=t.control_id JOIN body_chunk b ON b.control_id=c.id AND b.version_id=c.version_id
 WHERE c.version_id=ANY($1::bigint[]) AND c.id=ANY($2::bigint[]) AND b.id=ANY($3::bigint[])`
	args := []any{readable, controls, chunks}
	if lookup {
		sql = `SELECT t.control_id,t.title_original,t.body FROM control_text t JOIN control c ON c.id=t.control_id
 WHERE c.version_id=ANY($1::bigint[]) AND c.id=ANY($2::bigint[])`
		args = args[:2]
	}
	rows, err := tx.Query(ctx, sql, args...)
	if err != nil {
		return nil, ErrUnavailable
	}
	defer rows.Close()
	result := map[int64]controlText{}
	for rows.Next() {
		var id int64
		var text controlText
		if rows.Scan(&id, &text.title, &text.body) != nil {
			return nil, ErrUnavailable
		}
		result[id] = text
	}
	if rows.Err() != nil {
		return nil, ErrUnavailable
	}
	return result, nil
}
