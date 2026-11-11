package retrieval

import (
	"context"
	"fmt"
	"strconv"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
)

// Read returns bounded pages of independent scopes from the engine's release.
func (e *Engine) Read(ctx context.Context, req ReadRequest) (ReadResult, error) {
	result := ReadResult{Release: e.release.Rel}
	if req.PageCap < 0 || req.PartSize < 0 || (cursor{PageCap: req.PageCap, PartSize: req.PartSize}).partSize() > (cursor{PageCap: req.PageCap}).pageCap() {
		return result, ErrInvalidReadLimits
	}
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
	cursors := make([]cursor, len(req.Items))
	result.Items = make([]ItemResult, len(req.Items))
	pending := false
	for i, item := range req.Items {
		c, code := requestCursor(item, req)
		cursors[i] = c
		result.Items[i].Error = code
		pending = pending || code == ""
	}
	if !pending {
		return result, nil
	}
	ctx, cancel := context.WithTimeout(ctx, e.deadline())
	defer cancel()
	tx, err := e.db.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly, IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return result, databaseFailure("begin read transaction", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if err = setLocal(ctx, tx, "statement_timeout", strconv.FormatInt(e.deadline().Milliseconds(), 10)); err != nil {
		return result, fmt.Errorf("set read deadline: %w", err)
	}
	used := 0
	var saves []func()
	for i, c := range cursors {
		if result.Items[i].Error != "" {
			continue
		}
		page, cost, save, readErr := e.readItem(ctx, tx, c, req.Items[i].Next != "", c.pageCap()-used, i == 0)
		if readErr != nil {
			return ReadResult{}, fmt.Errorf("read item: %w", readErr)
		}
		used += cost
		if save != nil {
			saves = append(saves, save)
		}
		result.Items[i] = page
	}
	if err = tx.Commit(ctx); err != nil {
		return ReadResult{}, databaseFailure("commit read transaction", err)
	}
	if ctx.Err() == nil {
		for _, save := range saves {
			save()
		}
	}
	return result, nil
}

func validReadItem(item ReadItem) bool {
	if !validReadText(item.Handle, ReadHandleMax) || !validReadText(item.From, ReadHandleMax) || !validReadText(item.To, ReadHandleMax) || !validReadText(item.Number, ReadNumberMax) {
		return false
	}
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

func validReadText(s string, limit int) bool {
	for count := 0; s != ""; count++ {
		r, size := utf8.DecodeRuneInString(s)
		if count >= limit || r == 0 || (r == utf8.RuneError && size == 1) {
			return false
		}
		s = s[size:]
	}
	return true
}

func readCursor(item ReadItem, release string) (cursor, string) {
	if !validReadItem(item) {
		return cursor{}, "invalid_item"
	}
	c := cursor{V: 1, Release: release, Item: item}
	if item.Next != "" {
		var err error
		c, err = decodeNext(item.Next)
		if err != nil {
			return cursor{}, "invalid_next"
		}
		if c.Release != release {
			return cursor{}, "release_expired"
		}
		if !validReadItem(c.Item) || c.Item.Next != "" {
			return cursor{}, "invalid_next"
		}
	}
	return c, ""
}

func (e *Engine) readItem(ctx context.Context, tx pgx.Tx, c cursor, continued bool, budget int, first bool) (ItemResult, int, func(), error) {
	seq, code, hit, err := e.sequence(ctx, tx, c)
	if err != nil {
		return ItemResult{}, 0, nil, err
	}
	if code != "" {
		if continued {
			code = "invalid_next"
		}
		return ItemResult{Doc: seq.doc, Error: code}, 0, nil, nil
	}
	if e.sequenceCache(ctx) != nil && seq.parts == nil {
		seq.parts = make(map[int][]Unit)
	}
	local := c.Unit - seq.offset
	emptyDocument := seq.document != nil && len(seq.units) == 0 && c.Unit == 0 && c.Part == 0
	if continued && !emptyDocument {
		if local < 0 || local >= len(seq.units) {
			return ItemResult{Doc: seq.doc, Error: "invalid_next"}, 0, nil, nil
		}
		parts, ok := sequenceParts(seq, local, c)
		if (ok && c.Part >= len(parts)) || (!ok && c.Part != 0) {
			return ItemResult{Doc: seq.doc, Error: "invalid_next"}, 0, nil, nil
		}
	}
	if seq.document != nil {
		e.statusDocument(seq.document, seq.source)
	}
	page, cost, err := readPage(seq, c, budget, first)
	var save func()
	if !hit && err == nil && page.Error == "" {
		save = e.saveSequence(ctx, c, seq)
	}
	return page, cost, save, err
}

func readPage(seq readSequence, c cursor, budget int, first bool) (ItemResult, int, error) {
	result := ItemResult{Doc: seq.doc, Gaps: seq.gaps, FetchSHA256: seq.source.FetchSHA256}
	used := 0
	var firstParts []Unit
	firstPartsChecked, firstPartsOK := false, false
	if c.Unit == 0 && c.Part == 0 && seq.document != nil {
		result.Document = seq.document
		used = c.size(seq.document)
		if used > c.pageCap() {
			return ItemResult{Doc: seq.doc, Gaps: seq.gaps, Error: "unit_too_large"}, 0, nil
		}
		if len(seq.units) > 0 && used > c.pageCap()-c.partSize() {
			firstParts, firstPartsOK = sequenceParts(seq, 0, c)
			firstPartsChecked = true
		}
		if firstPartsOK && used+c.size(firstParts[0]) > c.pageCap() {
			return ItemResult{Doc: seq.doc, Gaps: seq.gaps, Error: "unit_too_large"}, 0, nil
		}
		if used > budget && !first {
			return ItemResult{Doc: seq.doc, NotRead: "call_cap", Next: encodeNext(c)}, 0, nil
		}
	}
	for i := c.Unit - seq.offset; i < len(seq.units); i++ {
		parts, ok := firstParts, firstPartsOK
		if i != 0 || !firstPartsChecked {
			parts, ok = sequenceParts(seq, i, c)
		}
		if !ok {
			result.Error = "unit_too_large"
			return result, used, nil
		}
		start := 0
		if i+seq.offset == c.Unit {
			start = c.Part
		}
		for j := start; j < len(parts); j++ {
			cost := c.size(parts[j])
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
