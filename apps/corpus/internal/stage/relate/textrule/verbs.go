package textrule

import "strings"

func (c *cursor) verb() string {
	switch {
	case c.take("sửa", "đổi"), c.take("bổ", "sung"):
		return "amends_supplements"
	case c.take("bãi", "bỏ"), c.take("hủy", "bỏ"):
		return "repeals"
	case c.take("thay", "thế"):
		return "replaces"
	case c.take("tạm", "ngưng", "hiệu", "lực"), c.take("ngưng", "hiệu", "lực"), c.take("tạm", "hoãn", "thi", "hành"):
		if !c.take("thi", "hành") {
			c.take("áp", "dụng")
		}
		return "suspends"
	default:
		return ""
	}
}

func (c *cursor) verbs() (kind, phrase, gap string) {
	start := c.pos
	kind = c.verb()
	if kind == "" {
		return "", "", ""
	}
	for {
		separator := c.pos
		if !c.take(",") && !c.take("và") {
			break
		}
		next := c.verb()
		if next == "" {
			c.pos = separator
			break
		}
		if next != kind {
			return kind, "", "relation_text_mixed_verbs"
		}
	}
	return kind, strings.ToLower(c.raw(start, c.pos)), ""
}
