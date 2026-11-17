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
		{"projected", `SELECT id,version_id,control_id,ordinal,token_count,1-(embedding <=> $2::vector)
 FROM body_chunk WHERE version_id=ANY($1::bigint[])`},
		{"public", `SELECT id,version_id,control_id,0,token_count,1-(embedding <=> $2::vector)
 FROM public_chunk WHERE version_id=ANY($1::bigint[])`},
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
