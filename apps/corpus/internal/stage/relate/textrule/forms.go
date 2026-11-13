package textrule

import "strings"

func (c *cursor) objects(kind, phrase string) ([]string, string, string) {
	if c.at("cụm") || c.at("từ") {
		return c.phraseObjects(phrase)
	}
	if phrase == "bỏ" {
		return nil, "", "relation_text_grammar"
	}
	if c.take("tên") {
		if kind != "amends_supplements" && kind != "replaces" {
			return nil, "", "relation_text_grammar"
		}
		c.take("của")
		anchors, ok := c.locators()
		if !ok {
			return nil, "", "relation_text_grammar"
		}
		return anchors, "amends_supplements", ""
	}
	probe := *c
	if probe.take("phụ", "lục") || probe.take("mẫu") || probe.take("biểu", "mẫu") || probe.take("chương") || probe.take("mục") || probe.take("phần") {
		return nil, "", "relation_text_unsupported_locator"
	}
	if phrase == "bổ sung" {
		probe := *c
		if article, ok := probe.newLocators(); ok && probe.take("vào") {
			sibling := probe.take("sau") || probe.take("trước")
			if !sibling {
				probe.take("cuối")
			}
			anchors, ok := probe.locators()
			if !ok {
				return nil, "", "relation_text_grammar"
			}
			if sibling {
				if article {
					return nil, "", "relation_text_document_level"
				}
				for i, a := range anchors {
					end := strings.LastIndex(a, "/")
					if end < 0 {
						return nil, "", "relation_text_document_level"
					}
					anchors[i] = a[:end]
				}
			}
			*c = probe
			return anchors, "amends_supplements", ""
		}
	}
	anchors, ok := c.locators()
	if !ok {
		return nil, "", "relation_text_grammar"
	}
	return anchors, "", ""
}

func (c *cursor) newLocators() (article, ok bool) {
	for {
		point := c.take("điểm")
		if !point && !c.take("khoản") {
			if !c.take("điều") {
				return false, false
			}
			article = true
		}
		if _, ok := c.label(point); !ok {
			return false, false
		}
		if !c.separator() {
			return article, true
		}
	}
}

func (c *cursor) phraseObjects(phrase string) ([]string, string, string) {
	if phrase != "thay thế" && phrase != "bãi bỏ" && phrase != "bỏ" {
		return nil, "", "relation_text_grammar"
	}
	c.take("cụm")
	if !c.take("từ") || !c.quotation() {
		return nil, "", "relation_text_grammar"
	}
	if phrase == "thay thế" {
		if !c.take("bằng") {
			return nil, "", "relation_text_grammar"
		}
		c.take("cụm")
		if !c.take("từ") || !c.quotation() {
			return nil, "", "relation_text_grammar"
		}
	}
	if !c.take("tại") {
		return nil, "", "relation_text_grammar"
	}
	var anchors []string
	for {
		if c.take("tên") {
			c.take("của")
		}
		group, ok := c.group()
		if !ok {
			return nil, "", "relation_text_grammar"
		}
		anchors = append(anchors, group...)
		if !c.separator() {
			break
		}
		c.take("tại")
	}
	return anchors, "amends_supplements", ""
}

func (c *cursor) quotation() bool {
	if c.pos >= len(c.tokens) || !c.tokens[c.pos].quoted {
		return false
	}
	end := quotationEnd(c.text, c.tokens[c.pos].start)
	if end < 0 {
		return false
	}
	for c.pos < len(c.tokens) && c.tokens[c.pos].start < end {
		c.pos++
	}
	return true
}
