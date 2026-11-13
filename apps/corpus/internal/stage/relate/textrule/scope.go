package textrule

func (c *cursor) articleMarker() {
	start := c.pos
	if !c.take("điều") {
		return
	}
	if _, ok := c.label(false); !ok || !c.take(".") {
		c.pos = start
	}
}

func (c *cursor) scopeNoun() bool {
	return c.take("điều", "khoản") || c.take("từ", "ngữ") || c.take("mẫu", "biểu") || c.take("biểu", "mẫu") || c.take("phụ", "lục") || c.take("cụm", "từ") || c.take("nội", "dung") || c.take("quy", "định") || c.take("điều") || c.take("khoản") || c.take("điểm") || c.take("từ") || c.take("mẫu")
}

func scopePrefix(c *cursor) bool {
	c.articleMarker()
	if c.verb() == "" {
		return false
	}
	for {
		start := c.pos
		if !c.take(",") && !c.take("và") {
			break
		}
		if c.verb() == "" {
			c.pos = start
			break
		}
	}
	if !c.take("một", "số") {
		c.take("các")
	}
	if !c.scopeNoun() {
		return false
	}
	for c.take(",") || c.take("và") {
		if !c.scopeNoun() {
			return false
		}
	}
	return c.take("của") || c.take("tại")
}

// ScopeLayout identifies a scope-list lead and whether the text holds only that
// lead. It does not check the target's identity.
func ScopeLayout(text string) (scoped, leadOnly bool) {
	r, balanced := readUnits([]Unit{{Text: text, Kind: "dieu", Close: true}})
	if !balanced { // Source layout still takes precedence over quotation checks.
		r.mask = make([]bool, len(r.text))
	}
	parts := r.scan()
	if len(parts) == 0 {
		return false, false
	}
	c := parts[0]
	return scopePrefix(&c), len(parts) == 1
}

func documentTarget(c *cursor, issuers map[string]string) (Target, string) {
	docType := c.documentType()
	if docType == "" {
		return Target{}, "relation_text_grammar"
	}
	c.take("số")
	number, ok := c.number()
	if !ok {
		return Target{}, "relation_text_grammar"
	}
	issuer, gap := checkNumber(number, docType, issuers)
	if gap != "" {
		return Target{}, gap
	}
	date, gap := c.tail()
	return Target{Number: number, IssuerCode: issuer, StatedIssueDate: date}, gap
}

func namesDocument(c cursor) bool {
	for c.pos < len(c.tokens) {
		probe := c
		if probe.documentType() != "" {
			probe.take("số")
			if _, ok := probe.number(); ok {
				return true
			}
		}
		c.pos++
	}
	return false
}
