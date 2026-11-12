package textrule

// Parse extracts relation sentences from one article using a closed issuer table.
func Parse(text string, issuers map[string]string) (matches []Match, gaps []Gap) {
	text = normalize(text)
	mask, balanced := quoteMask(text)
	if !balanced {
		return nil, []Gap{{Name: "relation_text_unbalanced_quote", Detail: text}}
	}
	for _, sentence := range sentences(text, mask) {
		match, gap := parseSentence(sentence, issuers)
		if gap != "" {
			gaps = append(gaps, Gap{Name: gap, Detail: sentence.text})
		} else if match.Kind != "" {
			matches = append(matches, match)
		}
	}
	return matches, gaps
}

func parseSentence(c cursor, issuers map[string]string) (Match, string) {
	c.listMarker()
	kind, phrase, gap := c.verbs()
	if gap != "" || kind == "" {
		return Match{}, gap
	}
	if !c.dateRange() {
		return Match{}, "relation_text_grammar"
	}
	anchors, ok := c.locators()
	if !ok || (!c.take("của") && !c.take("tại")) {
		return Match{}, "relation_text_grammar"
	}
	docType := c.documentType()
	if docType == "" {
		return Match{}, "relation_text_grammar"
	}
	c.take("số")
	number, ok := c.number()
	if !ok {
		return Match{}, "relation_text_grammar"
	}
	issuer, gap := checkNumber(number, docType, issuers)
	if gap != "" {
		return Match{}, gap
	}
	date, gap := c.tail()
	if gap != "" {
		return Match{}, gap
	}
	targets := make([]Target, 0, len(anchors))
	for _, anchor := range anchors {
		targets = append(targets, Target{Anchor: anchor, Number: number, IssuerCode: issuer, StatedIssueDate: date})
	}
	return Match{Kind: kind, VerbPhrase: phrase, Sentence: c.text, Targets: targets}, ""
}
