package retrieval

import "encoding/json"

func (c cursor) pageCap() int {
	if c.PageCap > 0 {
		return c.PageCap
	}
	return ReadCallCap
}
func (c cursor) partSize() int {
	if c.PartSize > 0 {
		return c.PartSize
	}
	return PartSize
}
func (c cursor) size(v any) int {
	if !c.WholeDocument {
		return readSize(v)
	}
	raw, _ := json.Marshal(v)
	return len(raw)
}

func requestCursor(item ReadItem, req ReadRequest) (cursor, string) {
	c, code := readCursor(item, req.Release)
	if code != "" {
		return c, code
	}
	expected := cursor{PageCap: req.PageCap, PartSize: req.PartSize, WholeDocument: req.WholeDocument}
	if item.Next != "" {
		if c.pageCap() != expected.pageCap() || c.partSize() != expected.partSize() || c.WholeDocument != req.WholeDocument {
			return cursor{}, "invalid_next"
		}
	} else {
		c.PageCap, c.PartSize, c.WholeDocument = req.PageCap, req.PartSize, req.WholeDocument
	}
	if req.WholeDocument {
		key, anchor, err := ParseHandle(c.Item.Handle)
		if err != nil || anchor != "" || c.Item.Number != "" || c.Item.Relations || c.Item.From != "" || (req.DocumentKey != "" && key != req.DocumentKey) {
			if item.Next != "" {
				return cursor{}, "invalid_next"
			}
			return cursor{}, "invalid_item"
		}
	}
	return c, ""
}
