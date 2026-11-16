package build

import (
	"context"
	"fmt"
	"slices"
	"sort"

	"danny.vn/trachuan/internal/frameworks/relate"
	"github.com/jackc/pgx/v5"
)

func edgeVersion(ids databaseIDs, key relate.Key) *int64 {
	if _, ok := ids.controls[key]; !ok {
		return nil
	}
	id, ok := ids.versions[versionKey(key.Code, key.Version)]
	if !ok {
		return nil
	}
	return &id
}

func evidenceDependencies(d databaseInputs, ids databaseIDs, edge relate.Edge) ([]int64, error) {
	access := map[string]string{}
	for _, v := range d.registered {
		access[versionKey(v.Code, v.Version)] = v.Access
	}
	want := []string{}
	for _, end := range []relate.Key{edge.From, edge.To} {
		key := versionKey(end.Code, end.Version)
		class, ok := access[key]
		if !ok {
			return nil, fmt.Errorf("evidence version not in snapshot")
		}
		if class != "free" && class != "licensed" {
			return nil, fmt.Errorf("unknown evidence access")
		}
		if class == "licensed" {
			want = append(want, "fw:"+key)
		}
	}
	sort.Strings(want)
	want = slices.Compact(want)
	if !slices.Equal(want, edge.Evidence.Requires) {
		return nil, fmt.Errorf("evidence dependency mismatch")
	}
	deps := make([]int64, 0, len(want))
	for _, handle := range want {
		id, err := requiredID(ids.versions, handle[3:])
		if err != nil {
			return nil, err
		}
		deps = append(deps, id)
	}
	slices.Sort(deps)
	return deps, nil
}

func loadEdges(ctx context.Context, tx pgx.Tx, d databaseInputs, ids databaseIDs) error {
	for i, edge := range d.edges {
		from, to := edgeVersion(ids, edge.From), edgeVersion(ids, edge.To)
		if edge.Resolved != (from != nil && to != nil) {
			return fmt.Errorf("mapping resolution mismatch")
		}
		id := i + 1
		_, err := tx.Exec(ctx, `INSERT INTO control_mapping(id,from_code,from_version,from_citation,to_code,to_version,to_citation,relation,source,from_version_id,to_version_id,resolved) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, id, edge.From.Code, edge.From.Version, edge.From.Citation, edge.To.Code, edge.To.Version, edge.To.Citation, edge.Relation, edge.Source, from, to, edge.Resolved)
		if err != nil {
			return hide("load mapping", err)
		}
		if edge.Evidence == nil {
			continue
		}
		deps, err := evidenceDependencies(d, ids, edge)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO mapping_evidence(mapping_id,evidence,licensed_version_ids) VALUES($1,$2,$3)`, id, edge.Evidence.Value, deps); err != nil {
			return hide("load mapping evidence", err)
		}
	}
	return nil
}
