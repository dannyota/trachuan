package serve

import (
	"context"
	"encoding/json"
	"regexp"
	"time"

	schema "danny.vn/trachuan/internal/corpusdb/frameworks"
	"danny.vn/trachuan/internal/embedding"
	"danny.vn/trachuan/internal/retrieval"
	"danny.vn/trachuan/internal/retrieval/lexical"
	"github.com/jackc/pgx/v5/pgxpool"
)

func Open(ctx context.Context, db *pgxpool.Pool, embedder retrieval.Embedder, options Options) (*Engine, error) {
	if db == nil || embedder == nil || len(options.CursorKey) < 32 {
		return nil, ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	e := &Engine{db: db, embedder: embedder, key: append([]byte(nil), options.CursorKey...)}
	var kind string
	var schemaVersion int
	var sealed bool
	var dense, lex, retrievalRecipe []byte
	var steps [5]json.RawMessage
	err := db.QueryRow(ctx, `SELECT kind,rel,schema_version,versions_sealed,dense_recipe,lexical_recipe,retrieval_recipe,
 chunk_recipe,snapshot_recipe,parse_recipe,relate_recipe,embed_recipe FROM release WHERE singleton`).Scan(
		&kind, &e.release, &schemaVersion, &sealed, &dense, &lex, &retrievalRecipe, &steps[0], &steps[1], &steps[2], &steps[3], &steps[4])
	if err != nil || validateIdentity(kind, e.release, schemaVersion, sealed) != nil {
		return nil, ErrUnavailable
	}
	if decodeRecipe(dense, &e.dense) != nil || decodeRecipe(lex, &e.lexical) != nil || decodeRecipe(retrievalRecipe, &e.recipe) != nil || e.recipe.validate() != nil || e.lexical != lexical.Recipe1() {
		return nil, ErrUnavailable
	}
	for i, name := range []string{"chunk", "snapshot", "parse", "relate", "embed"} {
		if !validStep(steps[i], name) {
			return nil, ErrUnavailable
		}
	}
	running, err := embedder.Recipe(ctx)
	if err != nil || e.dense.Document != embedding.DocumentRecipe1() || e.dense.Query != embedding.QueryRecipe1() {
		return nil, ErrUnavailable
	}
	for _, table := range []string{"public.body_chunk", "public.public_chunk"} {
		var dimensions int
		err = db.QueryRow(ctx, `SELECT atttypmod FROM pg_attribute WHERE attrelid=$1::regclass AND attname='embedding' AND NOT attisdropped`, table).Scan(&dimensions)
		if err != nil || retrieval.ValidateDenseRecipe(e.dense, running, dimensions) != nil {
			return nil, ErrUnavailable
		}
	}
	if err = e.loadVersions(ctx); err != nil {
		return nil, err
	}
	if err = e.loadPartitions(ctx); err != nil {
		return nil, err
	}
	return e, nil
}

func validateIdentity(kind, rel string, schemaVersion int, sealed bool) error {
	if kind != "frameworks" || schemaVersion != schema.SchemaVersion || !sealed || !regexp.MustCompile(`^[0-9]{8}(_[0-9]{1,2})?$`).MatchString(rel) {
		return ErrUnavailable
	}
	return nil
}

func validStep(raw []byte, name string) bool {
	var step struct {
		Name       string                     `json:"name"`
		Version    int                        `json:"version"`
		Parameters map[string]json.RawMessage `json:"parameters"`
	}
	if decodeRecipe(raw, &step) != nil || step.Name != name || step.Parameters == nil {
		return false
	}
	wantVersion := 1
	if name == "snapshot" {
		wantVersion = 2
	}
	if name == "relate" {
		wantVersion = 4
	}
	if step.Version != wantVersion {
		return false
	}
	if name == "chunk" {
		var schemaName string
		var limit, prefix int
		return json.Unmarshal(step.Parameters["schema"], &schemaName) == nil && schemaName == "chunks/1" &&
			json.Unmarshal(step.Parameters["limit"], &limit) == nil && limit == 512 &&
			json.Unmarshal(step.Parameters["prefix_format_version"], &prefix) == nil && prefix == 1
	}
	return true
}

func (e *Engine) loadVersions(ctx context.Context) error {
	rows, err := e.db.Query(ctx, `SELECT v.id,v.framework_code,v.version,f.name,f.publisher,v.access,f.aliases,
 coalesce('fw:'||s.framework_code||':'||s.version,''),coalesce('fw:'||a.framework_code||':'||a.version,'')
 FROM framework_version v JOIN framework f ON f.code=v.framework_code
 LEFT JOIN framework_version s ON s.id=v.supersedes_id LEFT JOIN framework_version a ON a.id=v.amends_id
 ORDER BY v.framework_code COLLATE "C",v.released DESC,v.ordinal DESC`)
	if err != nil {
		return ErrUnavailable
	}
	defer rows.Close()
	for rows.Next() {
		var v version
		if rows.Scan(&v.id, &v.info.Code, &v.info.Version, &v.info.Name, &v.info.Publisher, &v.info.Access, &v.aliases, &v.info.Lineage.Supersedes, &v.info.Lineage.Amends) != nil {
			return ErrUnavailable
		}
		v.handle = "fw:" + v.info.Code + ":" + v.info.Version
		e.versions = append(e.versions, v)
	}
	if rows.Err() != nil {
		return ErrUnavailable
	}
	return nil
}
