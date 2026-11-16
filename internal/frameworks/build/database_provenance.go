package build

import (
	"context"
	"encoding/json"
	"time"

	schema "danny.vn/trachuan/internal/corpusdb/frameworks"
	"danny.vn/trachuan/internal/retrieval/lexical"
)

func stepInputHash(inputs map[string]*string) string {
	if inputs == nil {
		inputs = map[string]*string{}
	}
	data, _ := json.Marshal(inputs)
	return digest(data)
}

func initializeRelease(ctx context.Context, db schema.DB, rel string, steps map[string]stepProvenance) error {
	_, err := db.Exec(ctx, `INSERT INTO release(
 singleton,kind,rel,schema_version,dense_recipe,chunk_recipe,lexical_recipe,retrieval_recipe,
 snapshot_recipe,parse_recipe,relate_recipe,embed_recipe,
 snapshot_input_hash,parse_input_hash,relate_input_hash,chunk_input_hash,embed_input_hash,
 framework_count,version_count,control_count,built_at)
 VALUES(true,'frameworks',$1,1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,0,0,0,$15)`,
		rel, denseRecipe(), steps["chunk"].Recipe, lexical.Recipe1(), bootstrapRetrievalRecipe(),
		steps["snapshot"].Recipe, steps["parse"].Recipe, steps["relate"].Recipe, steps["embed"].Recipe,
		steps["snapshot"].InputHash, steps["parse"].InputHash, steps["relate"].InputHash, steps["chunk"].InputHash, steps["embed"].InputHash,
		time.Now().UTC())
	if err != nil {
		return hide("initialize frameworks release", err)
	}
	return nil
}
