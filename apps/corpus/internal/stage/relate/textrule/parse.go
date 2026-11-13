package textrule

// Parse extracts relation sentences from one article using a closed issuer table.
func Parse(text string, issuers map[string]string) ([]Match, []Gap) {
	return ParseUnits([]Unit{{Text: text, Kind: "dieu", Close: true}}, issuers, nil)
}

// ParseUnits reads an article in source order. Identity returns a version 1 gap
// for an unlinked or ambiguous document number; nil leaves identity to the caller.
func ParseUnits(units []Unit, issuers map[string]string, identity func(string) string) (matches []Match, gaps []Gap) {
	r, balanced := readUnits(units)
	if !balanced {
		return nil, []Gap{{Name: "relation_text_unbalanced_quote", Detail: normalize(r.text)}}
	}
	parts := r.scan()
	var scope Target
	var lead string
	scoped, rejected := false, false
	for i, c := range parts {
		if i == 0 {
			probe := c
			if scopePrefix(&probe) {
				scoped = true
				lead = c.text
				var gap string
				scope, gap = documentTarget(&probe, issuers)
				detail := c.text
				if gap == "" && identity != nil {
					gap = identity(scope.Number)
					if gap != "" {
						detail = scope.Number
					}
				}
				if gap != "" {
					rejected = true
					gaps = append(gaps, Gap{Name: gap, Detail: detail})
				}
				continue
			}
		}
		if quotedSentence(c) || punctuationOnly(c) {
			continue
		}
		if introducesWording(c) && (!wordingEnd(c.text) || !validWording(parts, i)) {
			return nil, []Gap{{Name: "relation_text_unquoted_wording", Detail: c.text}}
		}
		if c.broken {
			return nil, []Gap{{Name: "relation_text_grammar", Detail: c.text}}
		}
		probe := c
		probe.articleMarker()
		probe.listMarker()
		if !candidate(probe) {
			continue
		}
		if scoped && !c.top {
			gaps = append(gaps, Gap{Name: "relation_text_nested_scope", Detail: c.text})
			continue
		}
		item := scoped && !namesDocument(c)
		if item && rejected {
			gaps = append(gaps, Gap{Name: "relation_text_scope_rejected", Detail: c.text})
			continue
		}
		match, gap := parseCandidate(c, issuers, item, scope)
		if gap != "" {
			gaps = append(gaps, Gap{Name: gap, Detail: c.text})
			continue
		}
		if match.Kind != "" {
			if item {
				match.Sentence = lead + "\n" + c.text
			}
			matches = append(matches, match)
		}
	}
	return matches, gaps
}

func candidate(c cursor) bool {
	return c.verb() != "" || (c.take("bỏ") && (c.take("cụm", "từ") || c.take("từ")))
}

func parseCandidate(c cursor, issuers map[string]string, item bool, scope Target) (Match, string) {
	c.articleMarker()
	c.listMarker()
	kind, phrase, gap := c.verbs()
	if kind == "" && c.take("bỏ") {
		kind = "amends_supplements"
		phrase = "bỏ"
	}
	if gap != "" || kind == "" {
		return Match{}, gap
	}
	if !c.dateRange() {
		return Match{}, "relation_text_grammar"
	}
	anchors, override, gap := c.objects(kind, phrase)
	if gap != "" {
		return Match{}, gap
	}
	if override != "" {
		kind = override
	}
	var target Target
	if item {
		if c.take("như", "sau") {
			c.take(":")
		}
		if !c.finish() {
			return Match{}, "relation_text_grammar"
		}
		target = scope
	} else {
		if !c.take("của") && !c.take("tại") {
			return Match{}, "relation_text_grammar"
		}
		target, gap = documentTarget(&c, issuers)
		if gap != "" {
			return Match{}, gap
		}
	}
	targets := make([]Target, 0, len(anchors))
	for _, a := range anchors {
		next := target
		next.Anchor = a
		targets = append(targets, next)
	}
	return Match{Kind: kind, VerbPhrase: phrase, Sentence: c.text, Targets: targets}, ""
}

func (c *cursor) finish() bool {
	c.take(".")
	c.take(";")
	c.take(":")
	return c.pos == len(c.tokens)
}
