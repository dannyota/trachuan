package serve

import "strings"

type statementRule struct{ kind, sql string }

func projectionRules() map[string]statementRule {
	rules := []statementRule{
		{"transaction", `begin isolation level repeatable read read only`},
		{"transaction", `commit`},
		{"transaction", `rollback`},
		{"transaction", `SELECT set_config('statement_timeout','2000',true),
 set_config('plan_cache_mode','force_custom_plan',true),set_config('enable_seqscan','off',true),
 set_config('enable_indexscan','on',true),set_config('enable_bitmapscan','on',true)`},
		{"release", `SELECT kind,rel,schema_version,versions_sealed,dense_recipe,lexical_recipe,retrieval_recipe,
 chunk_recipe,snapshot_recipe,parse_recipe,relate_recipe,embed_recipe FROM release WHERE singleton`},
		{"catalog", `SELECT atttypmod FROM pg_attribute WHERE attrelid=$1::regclass AND attname='embedding' AND NOT attisdropped`},
		{"public", `SELECT v.id,v.framework_code,v.version,f.name,f.publisher,v.access,f.aliases,
 coalesce('fw:'||s.framework_code||':'||s.version,''),coalesce('fw:'||a.framework_code||':'||a.version,'')
 FROM framework_version v JOIN framework f ON f.code=v.framework_code
 LEFT JOIN framework_version s ON s.id=v.supersedes_id LEFT JOIN framework_version a ON a.id=v.amends_id
 ORDER BY v.framework_code COLLATE "C",v.released DESC,v.ordinal DESC`},
		{"public", `SELECT c.id,c.version_id,c.parent_id,c.ordinal,c.citation,c.citation_norm,c.kind,c.title,c.status,
 (SELECT count(*) FROM control_mapping m WHERE
 (m.from_version_id=c.version_id AND m.from_citation=c.citation) OR (m.to_version_id=c.version_id AND m.to_citation=c.citation))
 FROM control c WHERE c.version_id=ANY($1::bigint[])`},
		{"projected", `SELECT id,version_id,control_id,ordinal,token_count,1-(embedding <=> $2::vector)
 FROM body_chunk WHERE version_id=ANY($1::bigint[])`},
		{"public", `SELECT id,version_id,control_id,0,token_count,1-(embedding <=> $2::vector)
 FROM public_chunk WHERE version_id=ANY($1::bigint[])`},
		{"projected", `SELECT chunk_id,token,tf FROM body_posting WHERE version_id=ANY($1::bigint[])`},
		{"public", `SELECT chunk_id,token,tf FROM public_posting WHERE version_id=ANY($1::bigint[])`},
		{"public", `SELECT chunk_count,avg_length FROM public_lexical_stats`},
		{"public", `SELECT token,df FROM public_lexical_df ORDER BY token`},
		{"projected", `SELECT t.control_id,t.title_original,b.content FROM control_text t
 JOIN control c ON c.id=t.control_id JOIN body_chunk b ON b.control_id=c.id AND b.version_id=c.version_id
 WHERE c.version_id=ANY($1::bigint[]) AND c.id=ANY($2::bigint[]) AND b.id=ANY($3::bigint[])`},
		{"projected", `SELECT t.control_id,t.title_original,t.body FROM control_text t JOIN control c ON c.id=t.control_id
 WHERE c.version_id=ANY($1::bigint[]) AND c.id=ANY($2::bigint[])`},
		{"gaps", `SELECT g.kind,'fw:'||v.framework_code||':'||v.version,g.subject,g.detail
 FROM quality_gap g JOIN framework_version v ON v.id=g.version_id
 WHERE g.version_id=ANY($1::bigint[]) AND (g.class='public' OR (g.class='text' AND g.version_id=ANY($2::bigint[])))
 ORDER BY g.kind COLLATE "C",v.framework_code COLLATE "C",v.version COLLATE "C",g.subject COLLATE "C",g.detail COLLATE "C"`},
	}
	indexed := map[string]statementRule{}
	rules = append(rules, operationProjectionRules()...)
	for _, rule := range rules {
		indexed[normalizeSQL(rule.sql)] = rule
	}
	return indexed
}

func normalizeSQL(sql string) string { return strings.Join(strings.Fields(sql), " ") }
