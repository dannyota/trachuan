package serve

import (
	"context"
	"slices"

	"danny.vn/trachuan/internal/frameworks/businesskey"
)

type control struct {
	id, versionID               int64
	parent                      *int64
	ordinal, mappings           int
	citation, norm, kind, title string
	status                      string
	supersedes, amends          *int64
	lineage                     Lineage
	path                        []string
	order                       []int
}

const controlsSQL = `SELECT id,version_id,parent_id,ordinal,citation,citation_norm,kind,title,status,supersedes_id,amends_id FROM control`

func (e *Engine) loadControls(ctx context.Context) error {
	result := map[int64]*control{}
	rows, err := e.db.Query(ctx, controlsSQL)
	if err != nil {
		return ErrUnavailable
	}
	defer rows.Close()
	for rows.Next() {
		c := &control{}
		if rows.Scan(&c.id, &c.versionID, &c.parent, &c.ordinal, &c.citation, &c.norm, &c.kind, &c.title, &c.status, &c.supersedes, &c.amends) != nil {
			return ErrUnavailable
		}
		if err := e.reserveControlMemory(c); err != nil {
			return err
		}
		part, ok := e.partitions[c.versionID]
		if !ok {
			return ErrUnavailable
		}
		part.controls[c.id] = c
		part.citations[c.citation] = c
		part.counts.Controls++
		result[c.id] = c
	}
	if rows.Err() != nil {
		return ErrUnavailable
	}
	for _, c := range result {
		path := []string{}
		order := []int{}
		seen := map[int64]bool{}
		for node := c; node != nil; {
			if seen[node.id] || node.versionID != c.versionID {
				return ErrUnavailable
			}
			seen[node.id] = true
			if err := e.reservePartitionMemory(int64(24 + len(node.citation))); err != nil {
				return err
			}
			order = append(order, node.ordinal)
			if node.id != c.id {
				path = append(path, node.citation)
			}
			if node.parent == nil {
				break
			}
			parent, exists := result[*node.parent]
			if !exists {
				return ErrUnavailable
			}
			node = parent
		}
		slices.Reverse(path)
		slices.Reverse(order)
		c.path, c.order = path, order
		links := [2]string{}
		for i, id := range []*int64{c.supersedes, c.amends} {
			if id == nil {
				continue
			}
			other, ok := result[*id]
			if !ok {
				return ErrUnavailable
			}
			if err := e.reservePartitionMemory(int64(len(e.partitions[other.versionID].handle) + 1 + 3*len(other.citation))); err != nil {
				return err
			}
			links[i] = e.partitions[other.versionID].handle + "#" + encodeCitation(other.citation)
		}
		c.lineage = Lineage{links[0], links[1]}
	}
	return nil
}

func encodeCitation(citation string) string { return businesskey.EncodeCitation(citation) }

func (e *Engine) controlsFor(versions []int64) map[int64]*control {
	result := map[int64]*control{}
	for _, id := range versions {
		for key, c := range e.partitions[id].controls {
			result[key] = c
		}
	}
	return result
}
