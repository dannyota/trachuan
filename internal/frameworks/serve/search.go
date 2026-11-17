package serve

import (
	"context"
	"errors"
	"math"
	"sort"
	"time"

	"danny.vn/trachuan/internal/embedding"
	"github.com/jackc/pgx/v5"
)

const transactionSettingsSQL = `SELECT set_config('statement_timeout','2000',true),
 set_config('plan_cache_mode','force_custom_plan',true),set_config('enable_seqscan','off',true),
 set_config('enable_indexscan','on',true),set_config('enable_bitmapscan','on',true)`

func (e *Engine) Search(ctx context.Context, request Request, caller Caller) (Response, error) {
	return e.search(ctx, request, caller, nil)
}

func (e *Engine) search(ctx context.Context, request Request, caller Caller, observation *calibrationObservation) (Response, error) {
	p := e.project(caller)
	response := Response{Release: e.release, AccessSetVersion: p.version, AccessUnavailable: caller.AccessUnavailable}
	r, err := e.validate(request)
	if err != nil {
		return response, err
	}
	position, err := e.cursorPosition(r.Cursor, r, p)
	if err != nil {
		return response, err
	}
	if e.db == nil || e.embedder == nil {
		return response, ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	tx, err := e.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return response, ErrUnavailable
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if _, err = tx.Exec(ctx, transactionSettingsSQL); err != nil {
		return response, ErrUnavailable
	}
	ids := e.selectVersions(r)
	controls := e.controlsFor(ids)
	lookup := e.lookup(r.Query, controls)
	rankedHits := lookup
	if len(lookup) == 0 {
		vector, embedErr := e.embed(ctx, r.Query)
		if embedErr != nil {
			return response, embedErr
		}
		allIDs := e.selectVersions(Request{})
		allControls := e.controlsFor(allIDs)
		chunks, collectErr := e.collect(ctx, tx, allIDs, p, vector.Values, r.Query, allControls)
		if collectErr != nil {
			return response, collectErr
		}
		rankedHits = e.rankFiltered(chunks, allControls, controls)
		if observation == nil {
			e.abstention(&response, rankedHits)
		} else if len(rankedHits) > 0 {
			observation.cosine = &rankedHits[0].cosine
		}
	}
	if observation != nil {
		observation.lookup = len(lookup) > 0
	}
	if position > len(rankedHits) {
		return response, ErrInvalidRequest
	}
	page := rankedHits[position:min(len(rankedHits), position+r.Limit)]
	hits, frameworks, err := e.hydrate(ctx, tx, page, controls, p, len(lookup) > 0)
	if err != nil {
		return response, err
	}
	gaps, err := e.searchGaps(ctx, tx, ids, p)
	if err != nil {
		return response, err
	}
	if tx.Commit(ctx) != nil {
		return response, ErrUnavailable
	}
	response.Hits, response.Frameworks, response.Gaps = hits, frameworks, gaps
	if next := position + len(page); next < len(rankedHits) {
		response.NextCursor = e.encodeCursor(r, p, next)
	}
	return response, nil
}

func (e *Engine) lookup(query string, controls map[int64]*control) []ranked {
	versions := map[int64]string{}
	for _, v := range e.versions {
		versions[v.id] = v.info.Code
	}
	for _, route := range e.citations(query) {
		var result []ranked
		for _, c := range controls {
			if versions[c.versionID] == route.code && c.norm == route.citation {
				result = append(result, ranked{controlID: c.id})
			}
		}
		if len(result) > 0 {
			sort.Slice(result, func(i, j int) bool { return e.less(controls[result[i].controlID], controls[result[j].controlID]) })
			return result
		}
	}
	return nil
}

func (e *Engine) embed(ctx context.Context, query string) (embedding.Vector, error) {
	v, err := e.embedder.Embed(ctx, query)
	if err != nil {
		var service *embedding.Error
		if errors.As(err, &service) {
			if service.Status == 429 || service.Code == "busy" {
				return v, ErrRateLimited
			}
			if service.Code == "query_too_long" || service.Code == "input_too_long" {
				return v, ErrInvalidRequest
			}
		}
		return v, ErrUnavailable
	}
	return v, validateVector(v, e.dense.Query)
}

func validateVector(v embedding.Vector, r embedding.Recipe) error {
	if v.TokenCount > r.TokenCap {
		return ErrInvalidRequest
	}
	if len(v.Values) != r.Dimensions || v.RecipeHash != r.Hash() || v.TokenCount <= 0 {
		return ErrUnavailable
	}
	norm := 0.0
	for _, value := range v.Values {
		if !finite(float64(value)) {
			return ErrUnavailable
		}
		norm += float64(value) * float64(value)
	}
	if math.Abs(math.Sqrt(norm)-1) > .001 {
		return ErrUnavailable
	}
	return nil
}
