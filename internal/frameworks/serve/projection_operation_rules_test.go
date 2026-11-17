package serve

func operationProjectionRules() []statementRule {
	return []statementRule{
		{"projected", `SELECT t.title_original,t.body,t.notes,t.tables FROM control_text t JOIN control c ON c.id=t.control_id
 WHERE c.version_id=ANY($1::bigint[]) AND c.id=$2`},
		{"public", `SELECT m.id,m.from_code,m.from_version,m.from_citation,m.to_code,m.to_version,m.to_citation,
 m.relation,m.source,m.resolved,m.from_version_id,m.to_version_id
 FROM control_mapping m
 WHERE (($1::boolean AND ((m.from_code=$2 AND m.from_version=$3 AND m.from_citation=$4)
 OR (m.to_code=$2 AND m.to_version=$3 AND m.to_citation=$4)))
 OR (NOT $1::boolean AND m.from_code=$2 AND m.from_version=$3 AND m.to_code=$5 AND m.to_version=$6))
 AND (cardinality($7::text[])=0 OR m.relation=ANY($7::text[]))
 ORDER BY m.from_code,m.from_version,m.from_citation,m.to_code,m.to_version,m.to_citation,m.relation,m.source`},
		{"projected", `SELECT mapping_id,evidence FROM mapping_evidence
 WHERE licensed_version_ids <@ $1::bigint[] AND mapping_id=ANY($2::bigint[]) ORDER BY mapping_id`},
		{"projected", `SELECT version_id,name,text_source,text_complete FROM source_file
 WHERE version_id=ANY($1::bigint[]) ORDER BY version_id,name,sha256`},
		{"gaps", `SELECT g.kind,'fw:'||v.framework_code||':'||v.version,g.subject,g.detail
 FROM quality_gap g JOIN framework_version v ON v.id=g.version_id
 WHERE g.version_id=ANY($1::bigint[]) AND (g.class='public' OR (g.class='text' AND g.version_id=ANY($2::bigint[])))
 AND ($3::text='' OR g.kind=$3)
 AND (NOT $4::boolean OR (g.kind,('fw:'||v.framework_code||':'||v.version) COLLATE "C",g.subject,g.detail)>($5,$6,$7,$8))
 ORDER BY g.kind COLLATE "C",('fw:'||v.framework_code||':'||v.version) COLLATE "C",g.subject COLLATE "C",g.detail COLLATE "C" LIMIT 201`},
	}
}
