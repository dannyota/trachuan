package retrieval

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"
)

func (e *Engine) readScope(ctx context.Context, tx pgx.Tx, c cursor) (readSequence, string, error) {
	item := c.Item
	id, key, doc, source, code, err := e.readDocument(ctx, tx, item)
	if err != nil || code != "" {
		return readSequence{}, code, err
	}
	_, anchor, _ := strings.Cut(item.Handle, "#")
	nodes, err := readNodes(ctx, tx, id, key)
	if err != nil {
		return readSequence{}, "", err
	}
	target := -1
	for i, p := range nodes {
		if p.Identity.Anchor == anchor {
			target = i
			break
		}
	}
	if anchor != "" && target < 0 {
		return readSequence{doc: key}, "not_found", nil
	}
	if item.Relations {
		ids := []int64{}
		if target >= 0 {
			for _, p := range readDescendants(nodes, nodes[target].ID) {
				ids = append(ids, p.ID)
			}
		}
		seq, err := readRelations(ctx, tx, key, ids, anchor != "", c.Unit, e.release.RetrievalRecipe.RelationEvidenceRuneCap)
		seq.doc = key
		return seq, "", err
	}
	seq := readSequence{doc: key, source: source, complete: true}
	if anchor == "" && item.From == "" {
		seq.document = &doc
		seq.units = readOutline(nodes)
		if c.WholeDocument && len(nodes) > 0 {
			seq = readRange(seq, nodes, 0, len(nodes)-1)
		}
		return seq, "", nil
	}
	if item.From != "" {
		from, to := -1, -1
		for i, p := range nodes {
			if p.Identity.Anchor == item.From {
				from = i
			}
			if p.Identity.Anchor == item.To {
				to = i
			}
		}
		if from < 0 || to < from {
			return seq, "invalid_range", nil
		}
		seq = readRange(seq, nodes, from, to)
	} else {
		seq.units, seq.gaps = readLeads(nodes, nodes[target])
		subtree, err := readSubtree(ctx, tx, id, key, anchor)
		if err != nil {
			return seq, "", err
		}
		rendered := renderTree(subtree, nodes[target].ID)
		seq.units = append(seq.units, readTextUnit("provision", nodes[target], readAncestry(nodes, nodes[target]), string(rendered.text)))
		if rendered.ambiguous {
			seq.gaps = append(seq.gaps, readAmbiguity(nodes[target]))
		}
	}
	seq.complete = len(seq.gaps) == 0
	return seq, "", nil
}

func readRange(seq readSequence, nodes []Provision, from, to int) readSequence {
	leads, gaps := readLeads(nodes, nodes[from])
	seq.units = append(seq.units, leads...)
	seq.gaps = append(seq.gaps, gaps...)
	for _, p := range nodes[from : to+1] {
		text, ownGaps := readOwnText(nodes, p)
		seq.gaps = append(seq.gaps, ownGaps...)
		seq.units = append(seq.units, readTextUnit("provision", p, readAncestry(nodes, p), text))
	}
	seq.complete = len(seq.gaps) == 0
	return seq
}
