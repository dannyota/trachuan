package evaluate

import (
	"context"
	"slices"

	"github.com/jackc/pgx/v5"
)

func invariantProbeTargets(req EvaluationRequest) []string {
	targets := slices.Clone(req.ProbeTargets)
	seen := map[string]bool{}
	for _, c := range req.Controls {
		key := c.Code + ":" + c.Version
		if !seen[key] {
			targets = append(targets, controlHandle(c))
			seen[key] = true
		}
	}
	return targets
}
func mappingProbeTargets(ctx context.Context, conn *pgx.Conn) ([]string, error) {
	rows, err := conn.Query(ctx, `SELECT DISTINCT CASE WHEN m.from_version_id IS NOT NULL THEN m.from_code ELSE m.to_code END,CASE WHEN m.from_version_id IS NOT NULL THEN m.from_version ELSE m.to_version END,CASE WHEN m.from_version_id IS NOT NULL THEN m.from_citation ELSE m.to_citation END
 FROM framework_version v JOIN LATERAL (
 SELECT m.* FROM control_mapping m LEFT JOIN mapping_evidence e ON e.mapping_id=m.id
 WHERE v.id=ANY(e.licensed_version_ids) OR ((m.from_version_id=v.id OR m.to_version_id=v.id) AND e.mapping_id IS NULL)
 ORDER BY (e.mapping_id IS NOT NULL) DESC,m.id LIMIT 1
 ) m ON true WHERE v.access='licensed' ORDER BY 1`)
	if err != nil {
		return nil, hide("read invariant probe handles", err)
	}
	defer rows.Close()
	targets := []string{}
	for rows.Next() {
		var c Control
		if err = rows.Scan(&c.Code, &c.Version, &c.Citation); err != nil {
			return nil, hide("read invariant probe handle", err)
		}
		targets = append(targets, controlHandle(c))
	}
	if err = rows.Err(); err != nil {
		return nil, hide("read invariant probe handles", err)
	}
	return targets, nil
}
