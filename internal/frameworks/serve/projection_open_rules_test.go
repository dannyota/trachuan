package serve

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func openingProjectionRules() map[string]statementRule {
	rules := []statementRule{
		{"release", `SELECT kind,rel,schema_version,versions_sealed,dense_recipe,lexical_recipe,retrieval_recipe,
 chunk_recipe,snapshot_recipe,parse_recipe,relate_recipe,embed_recipe FROM release WHERE singleton`},
		{"catalog", `SELECT atttypmod FROM pg_attribute WHERE attrelid=$1::regclass AND attname='embedding' AND NOT attisdropped`},
		{"public", `SELECT v.id,v.framework_code,v.version,f.name,f.publisher,v.access,f.aliases,
 coalesce('fw:'||s.framework_code||':'||s.version,''),coalesce('fw:'||a.framework_code||':'||a.version,'')
 FROM framework_version v JOIN framework f ON f.code=v.framework_code
 LEFT JOIN framework_version s ON s.id=v.supersedes_id LEFT JOIN framework_version a ON a.id=v.amends_id
 ORDER BY v.framework_code COLLATE "C",v.released DESC,v.ordinal DESC`},
		{"public", `SELECT id,version_id,parent_id,ordinal,citation,citation_norm,kind,title,status,supersedes_id,amends_id FROM control`},
		{"public", `SELECT ends.code,ends.version,ends.citation,count(*) FILTER (WHERE ends.version_id IS NOT NULL),
 count(*) FILTER (WHERE ends.outgoing AND m.resolved),count(*) FILTER (WHERE ends.outgoing AND NOT m.resolved)
 FROM control_mapping m CROSS JOIN LATERAL (VALUES
 (m.from_code,m.from_version,m.from_citation,m.from_version_id,true),(m.to_code,m.to_version,m.to_citation,m.to_version_id,false))
 AS ends(code,version,citation,version_id,outgoing) GROUP BY ends.code,ends.version,ends.citation`},
		{"openIndex", `SELECT id,version_id,control_id,ordinal,token_count FROM body_chunk`},
		{"public", `SELECT id,version_id,control_id,0,token_count FROM public_chunk`},
		{"openIndex", `SELECT version_id,chunk_id,token,tf FROM body_posting`},
		{"public", `SELECT version_id,chunk_id,token,tf FROM public_posting`},
	}
	indexed := map[string]statementRule{}
	for _, rule := range rules {
		indexed[normalizeSQL(rule.sql)] = rule
	}
	return indexed
}

func TestProjectionGuard_SeparatesOpenAndRequest(t *testing.T) {
	classes := map[string]relationClass{
		"body_posting": textRelation, "public_posting": publicRelation,
		"body_chunk": textRelation, "public_chunk": publicRelation,
	}
	for _, sql := range []string{bodyPostingsSQL, publicPostingsSQL, bodyChunksSQL, publicChunksSQL} {
		read := queryRead{sql: sql, opening: true}
		require.NoError(t, checkProjection([]queryRead{read}, classes, nil))
		read.opening = false
		require.Error(t, checkProjection([]queryRead{read}, classes, []int64{1, 2, 3}))
	}
	for _, sql := range []string{
		`SELECT body FROM control_text`, `SELECT title_original FROM control_text`,
		`SELECT notes,tables FROM control_text`, `SELECT evidence FROM mapping_evidence`,
		`SELECT * FROM quality_gap`, readTextSQL, bodyDenseSQL,
		`SELECT id,version_id,control_id,ordinal,token_count,content FROM body_chunk`,
	} {
		require.Error(t, checkProjection([]queryRead{{sql: sql, opening: true}}, classes, []int64{1, 2, 3}))
	}
}
