package textrule

import "danny.vn/trachuan/apps/corpus/internal/anchor"

func (c *cursor) separator() bool {
	return c.take(",") || c.take("và") || c.take(";")
}

func (c *cursor) label(point bool) (string, bool) {
	if c.pos >= len(c.tokens) || c.tokens[c.pos].quoted {
		return "", false
	}
	word := c.tokens[c.pos].word
	if (point && !letter(word)) || (!point && !digits(word)) {
		return "", false
	}
	c.pos++
	return word, true
}

func (c *cursor) group() ([]string, bool) {
	var points, clauses []string
	for c.take("điểm") {
		label, ok := c.label(true)
		if !ok {
			return nil, false
		}
		points = append(points, "/"+anchor.Segment("diem", label))
		separated := c.separator()
		if (separated && !c.at("điểm")) || (!separated && !c.at("khoản") && !c.at("điều")) {
			return nil, false
		}
	}
	for c.take("khoản") {
		label, ok := c.label(false)
		if !ok {
			return nil, false
		}
		clause := "/" + anchor.Segment("khoan", label)
		if len(points) == 0 {
			clauses = append(clauses, clause)
		} else {
			for _, point := range points {
				clauses = append(clauses, clause+point)
			}
			points = nil
		}
		separated := c.separator()
		if (separated && !c.at("khoản")) || (!separated && !c.at("điều")) {
			return nil, false
		}
	}
	if !c.take("điều") {
		return nil, false
	}
	label, ok := c.label(false)
	if !ok {
		return nil, false
	}
	article := anchor.Segment("dieu", label)
	suffixes := clauses
	if len(clauses) == 0 {
		suffixes = points
	}
	if len(suffixes) == 0 {
		return []string{article}, true
	}
	anchors := make([]string, 0, len(suffixes))
	for _, suffix := range suffixes {
		anchors = append(anchors, article+suffix)
	}
	return anchors, true
}

func (c *cursor) locators() ([]string, bool) {
	var anchors []string
	for {
		group, ok := c.group()
		if !ok {
			return nil, false
		}
		anchors = append(anchors, group...)
		if !c.separator() {
			return anchors, true
		}
	}
}
