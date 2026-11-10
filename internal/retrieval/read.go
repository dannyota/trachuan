package retrieval

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
)

// Read returns bounded pages of independent scopes from the engine's release.
func (e *Engine) Read(ctx context.Context, req ReadRequest) (ReadResult, error) {
	result := ReadResult{Release: e.release.Rel}
	if len(req.Items) > ReadItemsMax {
		return result, ErrTooManyItems
	}
	if len(req.Items) == 0 {
		return result, nil
	}
	if req.Release != e.release.Rel {
		for range req.Items {
			result.Items = append(result.Items, ItemResult{Error: "release_expired"})
		}
		return result, nil
	}
	ctx, cancel := context.WithTimeout(ctx, e.deadline())
	defer cancel()
	tx, err := e.db.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly, IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return result, fmt.Errorf("begin read transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if err = setLocal(ctx, tx, "statement_timeout", strconv.FormatInt(e.deadline().Milliseconds(), 10)); err != nil {
		return result, fmt.Errorf("set read deadline: %w", err)
	}
	used := 0
	for i, item := range req.Items {
		page, cost, readErr := e.readItem(ctx, tx, item, ReadCallCap-used, i == 0)
		if readErr != nil {
			return ReadResult{}, fmt.Errorf("read item: %w", readErr)
		}
		used += cost
		result.Items = append(result.Items, page)
	}
	if err = tx.Commit(ctx); err != nil {
		return ReadResult{}, fmt.Errorf("commit read transaction: %w", err)
	}
	return result, nil
}

func validReadItem(item ReadItem) bool {
	if item.Next != "" {
		return item.Handle == "" && item.Number == "" && item.From == "" && item.To == "" && !item.Relations
	}
	if item.Number != "" {
		return item.Handle == "" && item.From == "" && item.To == "" && !item.Relations
	}
	if item.Handle == "" {
		return false
	}
	_, anchor, err := ParseHandle(item.Handle)
	if err != nil {
		return false
	}
	if item.From != "" || item.To != "" {
		return anchor == "" && item.From != "" && item.To != "" && !item.Relations
	}
	return true
}

func (e *Engine) readItem(ctx context.Context, tx pgx.Tx, item ReadItem, budget int, first bool) (ItemResult, int, error) {
	if !validReadItem(item) {
		return ItemResult{Error: "invalid_item"}, 0, nil
	}
	c := cursor{V: 1, Release: e.release.Rel, Item: item}
	continued := item.Next != ""
	if continued {
		var err error
		c, err = decodeNext(item.Next)
		if err != nil {
			return ItemResult{Error: "invalid_next"}, 0, nil
		}
		if c.Release != e.release.Rel {
			return ItemResult{Error: "release_expired"}, 0, nil
		}
		if !validReadItem(c.Item) || c.Item.Next != "" {
			return ItemResult{Error: "invalid_next"}, 0, nil
		}
	}
	seq, code, err := e.readScope(ctx, tx, c)
	if err != nil {
		return ItemResult{}, 0, err
	}
	if code != "" {
		if continued {
			code = "invalid_next"
		}
		return ItemResult{Error: code}, 0, nil
	}
	local := c.Unit - seq.offset
	emptyDocument := seq.document != nil && len(seq.units) == 0 && c.Unit == 0 && c.Part == 0
	if continued && !emptyDocument {
		if local < 0 || local >= len(seq.units) {
			return ItemResult{Error: "invalid_next"}, 0, nil
		}
		parts, ok := unitParts(seq.units[local])
		if (ok && c.Part >= len(parts)) || (!ok && c.Part != 0) {
			return ItemResult{Error: "invalid_next"}, 0, nil
		}
	}
	return readPage(seq, c, budget, first)
}

func readPage(seq readSequence, c cursor, budget int, first bool) (ItemResult, int, error) {
	result := ItemResult{Gaps: seq.gaps}
	used := 0
	if c.Unit == 0 && c.Part == 0 && seq.document != nil {
		result.Document = seq.document
		used = readSize(seq.document)
		if used > budget && !first {
			return ItemResult{NotRead: "call_cap", Next: encodeNext(c)}, 0, nil
		}
	}
	for i := c.Unit - seq.offset; i < len(seq.units); i++ {
		parts, ok := unitParts(seq.units[i])
		if !ok {
			result.Error = "unit_too_large"
			return result, used, nil
		}
		start := 0
		if i+seq.offset == c.Unit {
			start = c.Part
		}
		for j := start; j < len(parts); j++ {
			cost := readSize(parts[j])
			if used+cost > budget && (!first || len(result.Units) != 0) {
				c.Unit, c.Part = i+seq.offset, j
				result.Next = encodeNext(c)
				if len(result.Units) == 0 {
					result.NotRead = "call_cap"
					result.Document = nil
					used = 0
				}
				return result, used, nil
			}
			result.Units = append(result.Units, parts[j])
			used += cost
		}
	}
	if seq.more {
		c.Unit = seq.offset + len(seq.units)
		c.Part = 0
		result.Next = encodeNext(c)
	} else {
		result.Complete = seq.complete
	}
	return result, used, nil
}

func (e *Engine) readScope(ctx context.Context, tx pgx.Tx, c cursor) (readSequence, string, error) {
	item := c.Item
	id, key, doc, code, err := e.readDocument(ctx, tx, item)
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
		return readSequence{}, "not_found", nil
	}
	if item.Relations {
		ids := []int64{}
		if target >= 0 {
			for _, p := range readDescendants(nodes, nodes[target].ID) {
				ids = append(ids, p.ID)
			}
		}
		seq, err := readRelations(ctx, tx, key, ids, anchor != "", c.Unit, e.release.RetrievalRecipe.RelationEvidenceRuneCap)
		return seq, "", err
	}
	seq := readSequence{complete: true}
	if anchor == "" && item.From == "" {
		seq.document = &doc
		seq.units = readOutline(nodes)
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
		seq.units, seq.gaps = readLeads(nodes, nodes[from])
		for _, p := range nodes[from : to+1] {
			text, gaps := readOwnText(nodes, p)
			seq.gaps = append(seq.gaps, gaps...)
			seq.units = append(seq.units, readTextUnit("provision", p, readAncestry(nodes, p), text))
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
