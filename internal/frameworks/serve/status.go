package serve

import "context"

type StatusRequest struct{ Release string }

type Counts struct {
	Controls        int `json:"controls"`
	EdgesResolved   int `json:"edges_resolved"`
	EdgesUnresolved int `json:"edges_unresolved"`
}

type SourceStatus struct {
	Name         string `json:"name"`
	TextSource   string `json:"text_source"`
	TextComplete bool   `json:"text_complete"`
}

type VersionStatus struct {
	Counts
	Access   string         `json:"access"`
	Readable bool           `json:"readable"`
	Lineage  Lineage        `json:"lineage,omitzero"`
	Sources  []SourceStatus `json:"sources,omitempty"`
}

type StatusResponse struct {
	Metadata
	RetrievalRecipeStatus string                   `json:"retrieval_recipe_status"`
	Versions              map[string]VersionStatus `json:"versions,omitempty"`
	Totals                Counts                   `json:"totals"`
}

const statusSourcesSQL = `SELECT version_id,name,text_source,text_complete FROM source_file
 WHERE version_id=ANY($1::bigint[]) ORDER BY version_id,name,sha256`

func (e *Engine) CorpusStatus(ctx context.Context, r StatusRequest, caller Caller) (StatusResponse, error) {
	p := e.project(caller)
	response := StatusResponse{Metadata: e.metadata(p, caller), RetrievalRecipeStatus: e.recipe.Status}
	if !e.releaseMatches(r.Release) {
		return response, ErrReleaseExpired
	}
	ctx, tx, cancel, err := e.beginOperation(ctx)
	if err != nil {
		return response, err
	}
	defer cancel()
	defer func() { _ = tx.Rollback(context.Background()) }()
	handles := map[int64]string{}
	versions := map[string]VersionStatus{}
	for _, v := range e.versions {
		handles[v.id] = v.handle
		counts := e.partitions[v.id].counts
		versions[v.handle] = VersionStatus{Counts: counts, Access: v.info.Access, Readable: p.allowed[v.id], Lineage: v.info.Lineage}
		response.Totals.Controls += counts.Controls
		response.Totals.EdgesResolved += counts.EdgesResolved
		response.Totals.EdgesUnresolved += counts.EdgesUnresolved
	}
	if len(p.readable) > 0 {
		sources, sourceErr := tx.Query(ctx, statusSourcesSQL, p.readable)
		if sourceErr != nil {
			return response, ErrUnavailable
		}
		for sources.Next() {
			var id int64
			var source SourceStatus
			if sources.Scan(&id, &source.Name, &source.TextSource, &source.TextComplete) != nil {
				sources.Close()
				return response, ErrUnavailable
			}
			value := versions[handles[id]]
			value.Sources = append(value.Sources, source)
			versions[handles[id]] = value
		}
		err = sources.Err()
		sources.Close()
		if err != nil {
			return response, ErrUnavailable
		}
	}
	if tx.Commit(ctx) != nil {
		return response, ErrUnavailable
	}
	response.Versions = versions
	return response, nil
}
