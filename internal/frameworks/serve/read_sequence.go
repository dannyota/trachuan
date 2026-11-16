package serve

import (
	"context"
	"sort"

	"github.com/jackc/pgx/v5"
)

const readTextSQL = `SELECT t.title_original,t.body,t.notes,t.tables FROM control_text t JOIN control c ON c.id=t.control_id
 WHERE c.version_id=ANY($1::bigint[]) AND c.id=$2`

const readLineageSQL = `SELECT sv.framework_code,sv.version,s.citation,av.framework_code,av.version,a.citation
 FROM control c LEFT JOIN control s ON s.id=c.supersedes_id LEFT JOIN framework_version sv ON sv.id=s.version_id
 LEFT JOIN control a ON a.id=c.amends_id LEFT JOIN framework_version av ON av.id=a.version_id
 WHERE c.version_id=ANY($1::bigint[]) AND c.id=$2`

func (e *Engine) readSequence(ctx context.Context, tx pgx.Tx, cursor continuation, p projection) ([]ReadUnit, string, error) {
	h, _ := parseHandle(cursor.Handle)
	v, exists := e.versionFor(h)
	if !exists {
		return nil, "not_found", nil
	}
	controls, err := loadControls(ctx, tx, []int64{v.id})
	if err != nil {
		return nil, "", err
	}
	var target *control
	if h.citation != "" {
		for _, c := range controls {
			if c.citation == h.citation {
				target = c
				break
			}
		}
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
		lineage, lineageErr := controlLineage(ctx, tx, v.id, target.id)
		if lineageErr != nil {
			return nil, "", lineageErr
		}
		units = append(units, ReadUnit{Kind: "head", Head: &ReadHead{Handle: h.String(), Citation: target.citation, Kind: target.kind, Path: target.path, Title: target.title, Status: target.status, Lineage: lineage, Access: v.info.Access, TextWithheld: !p.allowed[v.id]}})
		if p.allowed[v.id] {
			var title, body string
			var notes, tables []string
			if tx.QueryRow(ctx, readTextSQL, p.readable, target.id).Scan(&title, &body, &notes, &tables) != nil {
				return nil, "", ErrUnavailable
			}
			units = append(units, textUnits(title, body, notes, tables)...)
		}
		rows, mappingErr := loadMappingRows(ctx, tx, MappingsRequest{Handle: h.String()})
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

func controlLineage(ctx context.Context, tx pgx.Tx, version, id int64) (Lineage, error) {
	var fields [6]*string
	if tx.QueryRow(ctx, readLineageSQL, []int64{version}, id).Scan(&fields[0], &fields[1], &fields[2], &fields[3], &fields[4], &fields[5]) != nil {
		return Lineage{}, ErrUnavailable
	}
	links := [2]string{}
	for i := range links {
		if fields[i*3] != nil && fields[i*3+1] != nil && fields[i*3+2] != nil {
			links[i] = (handle{*fields[i*3], *fields[i*3+1], *fields[i*3+2]}).String()
		}
	}
	return Lineage{links[0], links[1]}, nil
}
