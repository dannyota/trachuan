package serve

import (
	"context"

	"github.com/jackc/pgx/v5"
)

const mappingsSQL = `SELECT m.id,m.from_code,m.from_version,m.from_citation,m.to_code,m.to_version,m.to_citation,
 m.relation,m.source,m.resolved,fc.title,tc.title
 FROM control_mapping m LEFT JOIN control fc ON fc.version_id=m.from_version_id AND fc.citation=m.from_citation
 LEFT JOIN control tc ON tc.version_id=m.to_version_id AND tc.citation=m.to_citation
 WHERE (($1::boolean AND ((m.from_code=$2 AND m.from_version=$3 AND m.from_citation=$4)
 OR (m.to_code=$2 AND m.to_version=$3 AND m.to_citation=$4)))
 OR (NOT $1::boolean AND m.from_code=$2 AND m.from_version=$3 AND m.to_code=$5 AND m.to_version=$6))
 AND (cardinality($7::text[])=0 OR m.relation=ANY($7::text[]))
 ORDER BY m.from_code,m.from_version,m.from_citation,m.to_code,m.to_version,m.to_citation,m.relation,m.source`

const mappingEvidenceSQL = `SELECT mapping_id,evidence FROM mapping_evidence
 WHERE licensed_version_ids <@ $1::bigint[] AND mapping_id=ANY($2::bigint[]) ORDER BY mapping_id`

func loadMappingRows(ctx context.Context, tx pgx.Tx, r MappingsRequest) ([]mappingRow, error) {
	from, _ := parseHandle(r.From)
	to, _ := parseHandle(r.To)
	if r.Handle != "" {
		from, _ = parseHandle(r.Handle)
	}
	relations := r.Relations
	if relations == nil {
		relations = []string{}
	}
	rows, err := tx.Query(ctx, mappingsSQL, r.Handle != "", from.code, from.version, from.citation, to.code, to.version, relations)
	if err != nil {
		return nil, ErrUnavailable
	}
	defer rows.Close()
	var result []mappingRow
	for rows.Next() {
		var row mappingRow
		row.key = make([]string, 8)
		var fromTitle, toTitle *string
		if rows.Scan(&row.id, &row.key[0], &row.key[1], &row.key[2], &row.key[3], &row.key[4], &row.key[5], &row.key[6], &row.key[7], &row.value.Resolved, &fromTitle, &toTitle) != nil {
			return nil, ErrUnavailable
		}
		row.value.From = mappingEnd(row.key[0:3], fromTitle)
		row.value.To = mappingEnd(row.key[3:6], toTitle)
		row.value.Relation, row.value.Source = row.key[6], row.key[7]
		result = append(result, row)
	}
	if rows.Err() != nil {
		return nil, ErrUnavailable
	}
	return result, nil
}

func mappingEnd(key []string, title *string) MappingEnd {
	end := MappingEnd{Code: key[0], Version: key[1], Citation: key[2]}
	if title != nil {
		h := handle{key[0], key[1], key[2]}
		end.Handle, end.Version, end.Code, end.Title = h.String(), h.versionHandle(), "", *title
	}
	return end
}

func mappingEvidence(ctx context.Context, tx pgx.Tx, page []mappingRow, p projection) error {
	if len(page) == 0 {
		return nil
	}
	ids := make([]int64, len(page))
	indices := map[int64]int{}
	for i, row := range page {
		ids[i], indices[row.id] = row.id, i
	}
	readable := p.readable
	if readable == nil {
		readable = []int64{}
	}
	rows, err := tx.Query(ctx, mappingEvidenceSQL, readable, ids)
	if err != nil {
		return ErrUnavailable
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var evidence string
		if rows.Scan(&id, &evidence) != nil {
			return ErrUnavailable
		}
		i, found := indices[id]
		if !found {
			return ErrUnavailable
		}
		page[i].value.Evidence = evidence
	}
	if rows.Err() != nil {
		return ErrUnavailable
	}
	return nil
}
