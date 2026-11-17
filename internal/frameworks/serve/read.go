package serve

import (
	"context"
	"strings"
	"unicode/utf8"

	"danny.vn/trachuan/internal/retrieval"
)

func (e *Engine) readCursor(item ReadItem, release string, p projection) (continuation, string) {
	if (item.Handle == "") == (item.Next == "") {
		return continuation{}, "invalid_item"
	}
	c := continuation{Operation: "read", Release: e.release, Access: p.version, Handle: item.Handle}
	if item.Next != "" {
		decoded, err := e.unpackContinuation(item.Next)
		if err != nil {
			return c, "invalid_next"
		}
		digest := digestValue([]string{decoded.Handle, decoded.Mode})
		c, err = e.decodeContinuation(item.Next, "read", digest, p)
		if err != nil {
			return c, err.Error()
		}
	} else if !utf8.ValidString(item.Handle) || utf8.RuneCountInString(item.Handle) > retrieval.ReadHandleMax || strings.ContainsRune(item.Handle, 0) {
		return c, "invalid_item"
	}
	h, err := parseHandle(c.Handle)
	if err != nil {
		if item.Next != "" {
			return c, "invalid_next"
		}
		return c, "invalid_handle"
	}
	mode := "version"
	if h.citation != "" {
		mode = "control"
	}
	if item.Next != "" && (c.Mode != mode || len(c.Key) != 0) {
		return c, "invalid_next"
	}
	if !e.releaseMatches(release) {
		return c, "release_expired"
	}
	c.Mode, c.Digest = mode, digestValue([]string{c.Handle, mode})
	return c, ""
}

func (e *Engine) Read(ctx context.Context, request ReadRequest, caller Caller) (ReadResponse, error) {
	p := e.project(caller)
	response := ReadResponse{Metadata: e.metadata(p, caller)}
	if len(request.Items) == 0 || len(request.Items) > retrieval.ReadItemsMax {
		return response, ErrInvalidRequest
	}
	cursors := make([]continuation, len(request.Items))
	results := make([]ReadItemResult, len(request.Items))
	frameworks, laws, valid := false, false, false
	for i, item := range request.Items {
		if item.Next == "" && utf8.ValidString(item.Handle) && utf8.RuneCountInString(item.Handle) <= retrieval.ReadHandleMax && !strings.ContainsRune(item.Handle, 0) {
			_, frameworkErr := parseHandle(item.Handle)
			_, _, lawErr := retrieval.ParseHandle(item.Handle)
			frameworks = frameworks || frameworkErr == nil
			laws = laws || lawErr == nil
		}
		c, code := e.readCursor(item, request.Release, p)
		if code == "" && request.ExpectedHandle != "" && c.Handle != request.ExpectedHandle {
			code = "invalid_next"
		}
		cursors[i], results[i].Error = c, code
		if code == "" {
			frameworks, valid = true, true
		}
	}
	if frameworks && laws {
		return response, ErrInvalidRequest
	}
	if !valid {
		response.Items = results
		return response, nil
	}
	ctx, tx, cancel, err := e.beginOperation(ctx)
	if err != nil {
		return response, err
	}
	defer cancel()
	defer func() { _ = tx.Rollback(context.Background()) }()
	budget := retrieval.ReadCallCap
	for i, c := range cursors {
		if results[i].Error != "" {
			continue
		}
		units, code, loadErr := e.readSequence(ctx, tx, c, p)
		if loadErr != nil {
			return response, loadErr
		}
		if code != "" {
			if request.Items[i].Next != "" {
				code = "invalid_next"
			}
			results[i] = ReadItemResult{Handle: c.Handle, Error: code}
			continue
		}
		page, cost := e.readPage(units, c, p, budget, retrieval.PartSize)
		results[i], budget = page, budget-cost
	}
	if tx.Commit(ctx) != nil {
		return response, ErrUnavailable
	}
	response.Items = results
	return response, nil
}
