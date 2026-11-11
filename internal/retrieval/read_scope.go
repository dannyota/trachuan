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
		if i%128 == 0 {
			if err = ctx.Err(); err != nil {
				return readSequence{}, "", err
			}
		}
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
		relations, readErr := readRelations(ctx, tx, key, ids, anchor != "", c.Unit, e.release.RetrievalRecipe.RelationEvidenceRuneCap)
		relations.doc = key
		return relations, "", readErr
	}
	seq := readSequence{doc: key, source: source, complete: true}
	if anchor == "" && item.From == "" {
		seq.document = &doc
		seq.units, err = readOutlineContext(ctx, nodes)
		if err != nil {
			return readSequence{}, "", err
		}
		if c.WholeDocument && len(nodes) > 0 {
			seq, err = readRangeContext(ctx, seq, nodes, 0, len(nodes)-1)
			if err != nil {
				return readSequence{}, "", err
			}
		}
		return seq, "", nil
	}
	if item.From != "" {
		from, to := -1, -1
		for i, p := range nodes {
			if i%128 == 0 {
				if err = ctx.Err(); err != nil {
					return readSequence{}, "", err
				}
			}
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
		seq, err = readRangeContext(ctx, seq, nodes, from, to)
		if err != nil {
			return readSequence{}, "", err
		}
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

func readRangeContext(ctx context.Context, seq readSequence, nodes []Provision, from, to int) (readSequence, error) {
	tree, err := newReadTree(ctx, nodes)
	if err != nil {
		return readSequence{}, err
	}
	leads, gaps, err := tree.leads(ctx, from)
	if err != nil {
		return readSequence{}, err
	}
	seq.units = append(seq.units, leads...)
	seq.gaps = append(seq.gaps, gaps...)
	for i := from; i <= to; i++ {
		if i%128 == 0 {
			if err = ctx.Err(); err != nil {
				return readSequence{}, err
			}
		}
		if err = tree.render(ctx, i); err != nil {
			return readSequence{}, err
		}
		n := &tree.nodes[i]
		if n.ambiguous {
			seq.gaps = append(seq.gaps, readAmbiguity(n.provision))
		}
		p := n.provision
		seq.units = append(seq.units, Unit{Kind: "provision", Locator: p.Locator, Anchor: p.Identity.Anchor, Citation: n.citation, Label: p.Label, Heading: p.Heading, Text: n.own})
	}
	seq.complete = len(seq.gaps) == 0
	return seq, nil
}

func readRange(seq readSequence, nodes []Provision, from, to int) readSequence {
	result, _ := readRangeContext(context.Background(), seq, nodes, from, to)
	return result
}
