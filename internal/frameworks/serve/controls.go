package serve

import (
	"context"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"
)

type control struct {
	id, versionID               int64
	parent                      *int64
	ordinal, mappings           int
	citation, norm, kind, title string
	status                      string
	path                        []string
	order                       []int
}

func loadControls(ctx context.Context, tx pgx.Tx, versions []int64) (map[int64]*control, error) {
	result := map[int64]*control{}
	if len(versions) == 0 {
		return result, nil
	}
	rows, err := tx.Query(ctx, `SELECT c.id,c.version_id,c.parent_id,c.ordinal,c.citation,c.citation_norm,c.kind,c.title,c.status,
 (SELECT count(*) FROM control_mapping m WHERE
 (m.from_version_id=c.version_id AND m.from_citation=c.citation) OR (m.to_version_id=c.version_id AND m.to_citation=c.citation))
 FROM control c WHERE c.version_id=ANY($1::bigint[])`, versions)
	if err != nil {
		return nil, ErrUnavailable
	}
	defer rows.Close()
	for rows.Next() {
		c := &control{}
		if rows.Scan(&c.id, &c.versionID, &c.parent, &c.ordinal, &c.citation, &c.norm, &c.kind, &c.title, &c.status, &c.mappings) != nil {
			return nil, ErrUnavailable
		}
		result[c.id] = c
	}
	if rows.Err() != nil {
		return nil, ErrUnavailable
	}
	for _, c := range result {
		path := []string{}
		order := []int{}
		seen := map[int64]bool{}
		for node := c; node != nil; {
			if seen[node.id] || node.versionID != c.versionID {
				return nil, ErrUnavailable
			}
			seen[node.id] = true
			order = append(order, node.ordinal)
			if node.id != c.id {
				path = append(path, node.citation)
			}
			if node.parent == nil {
				break
			}
			parent, exists := result[*node.parent]
			if !exists {
				return nil, ErrUnavailable
			}
			node = parent
		}
		slices.Reverse(path)
		slices.Reverse(order)
		c.path, c.order = path, order
	}
	return result, nil
}

func encodeCitation(citation string) string {
	result := ""
	for i := 0; i < len(citation); i++ {
		b := citation[i]
		if (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z') || (b >= '0' && b <= '9') || b == '.' || b == '(' || b == ')' || b == '_' || b == '~' || b == '-' {
			result += string(b)
		} else {
			result += fmt.Sprintf("%%%02X", b)
		}
	}
	return result
}
