package serve

import (
	"context"
	"slices"
	"sort"

	"github.com/jackc/pgx/v5"
)

const readTextSQL = `SELECT t.title_original,t.body,t.notes,t.tables FROM control_text t JOIN control c ON c.id=t.control_id
 WHERE c.version_id=ANY($1::bigint[]) AND c.id=$2`

func (e *Engine) readSequence(ctx context.Context, tx pgx.Tx, cursor continuation, p projection) ([]ReadUnit, string, error) {
	h, _ := parseHandle(cursor.Handle)
	v, exists := e.versionFor(h)
	if !exists {
		return nil, "not_found", nil
	}
	controls := e.partitions[v.id].controls
	var target *control
	if h.citation != "" {
		target = e.partitions[v.id].citations[h.citation]
		if target == nil {
			return nil, "not_found", nil
		}
	}
	var units []ReadUnit
	if target == nil {
		info := v.info
		info.Readable = p.allowed[v.id]
		units = append(units, ReadUnit{Kind: "head", Version: &info})
	} else {
		units = append(units, ReadUnit{Kind: "head", Head: &ReadHead{Handle: h.String(), Citation: target.citation, Kind: target.kind, Path: slices.Clone(target.path), Title: target.title, Status: target.status, Lineage: target.lineage, Access: v.info.Access, TextWithheld: !p.allowed[v.id]}})
		if p.allowed[v.id] {
			var title, body string
			var notes, tables []string
			if tx.QueryRow(ctx, readTextSQL, p.readable, target.id).Scan(&title, &body, &notes, &tables) != nil {
				return nil, "", ErrUnavailable
			}
			units = append(units, textUnits(title, body, notes, tables)...)
		}
		rows, mappingErr := e.loadMappingRows(ctx, tx, MappingsRequest{Handle: h.String()})
		if mappingErr != nil {
			return nil, "", mappingErr
		}
		if mappingErr = mappingEvidence(ctx, tx, rows, p); mappingErr != nil {
			return nil, "", mappingErr
		}
		for _, row := range rows {
			edge := row.value
			units = append(units, ReadUnit{Kind: "mapping", Edge: &edge})
		}
	}
	var children []*control
	for _, c := range controls {
		if (target == nil && c.parent == nil) || (target != nil && c.parent != nil && *c.parent == target.id) {
			children = append(children, c)
		}
	}
	sort.Slice(children, func(i, j int) bool { return e.less(children[i], children[j]) })
	for _, child := range children {
		units = append(units, ReadUnit{Kind: "outline", Outline: &Outline{child.citation, child.kind, child.title}})
	}
	return units, "", nil
}
