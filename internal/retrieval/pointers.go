package retrieval

// Pointer identifies the other endpoint of a stored relation.
type Pointer struct {
	Dir       string `json:"dir,omitempty"`
	Kind      string `json:"kind,omitempty"`
	Key       string `json:"key,omitempty"`
	Number    string `json:"number,omitempty"`
	InRelease bool   `json:"in_release"`
}

func pointerKinds() []string {
	return []string{"repeals", "replaces", "suspends", "amends_supplements", "consolidates", "implements", "guided_by", "legal_basis", "cites", "references", "unknown"}
}

func pointerKindRank(kind string) int {
	for i, candidate := range pointerKinds() {
		if candidate == kind {
			return i
		}
	}
	return len(pointerKinds())
}

// The two branches retain both directions of a self-relation.
const incidentRelationsSQL = `
 SELECT d.key AS doc, 'in' AS dir, r.kind, r.source_key AS other_key,
        r.source_number AS other_number, other.key AS handle, other.issue_date,
        r.source_key, r.artifact_sha256, r.ordinal
 FROM document d JOIN relation r ON r.target_document_id = d.id
 LEFT JOIN document other ON other.id = r.source_document_id
 WHERE d.key = ANY($1)
 UNION ALL
 SELECT d.key, 'out', r.kind, r.target_key, r.target_number, other.key, other.issue_date,
        r.source_key, r.artifact_sha256, r.ordinal
 FROM document d JOIN relation r ON r.source_document_id = d.id
 LEFT JOIN document other ON other.id = r.target_document_id
 WHERE d.key = ANY($1)`

const documentCountsSQL = `WITH incident AS (` + incidentRelationsSQL + `)
 SELECT doc, dir, kind, count(*) FROM incident GROUP BY doc, dir, kind`

const documentPointersSQL = `WITH incident AS (` + incidentRelationsSQL + `), ranked AS (
 SELECT *, row_number() OVER (PARTITION BY doc ORDER BY
 COALESCE(array_position($2::text[], kind), cardinality($2::text[]) + 1),
 dir COLLATE "C", issue_date DESC NULLS LAST, other_number COLLATE "C",
 source_key COLLATE "C", artifact_sha256 COLLATE "C", ordinal) AS rank
 FROM incident)
 SELECT doc, dir, kind, COALESCE(handle, other_key, ''), other_number, handle IS NOT NULL
 FROM ranked WHERE rank <= $3 ORDER BY doc, rank`
