package textparse

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"unicode/utf8"
)

type span struct {
	start, end int
	kind       string
}

// Parse builds provisions and records uncertainty without asserting unverified articles.
func Parse(paragraphs []Paragraph) Result {
	r := Result{}
	for _, p := range paragraphs {
		n := utf8.RuneCountInString(p.Text)
		r.Counts.Characters += n
		if p.Text == "[hình]" {
			r.Counts.Images++
		}
	}
	r.Counts.Paragraphs = len(paragraphs)
	if r.Counts.Characters < 300 {
		r.Gaps = append(r.Gaps, "no_text")
		return r
	}
	quoted, quoteFailures := quotedParagraphs(paragraphs)
	spans := scopes(paragraphs, quoted)
	if len(spans) == 0 {
		r.Provisions = neutral(paragraphs, quoted, -1, 0)
		numberNeutral(r.Provisions)
		addContentGaps(&r, paragraphs)
		return r
	}
	r.Provisions = append(r.Provisions, preamble(paragraphs[:spans[0].start], quoted[:spans[0].start])...)
	instrumentNumber := 0
	failureIndex := 0
	for index, sp := range spans {
		for failureIndex < len(quoteFailures) && quoteFailures[failureIndex] < sp.start {
			failureIndex++
		}
		scope := Scope{Kind: sp.kind}
		var nums []string
		structures := map[string]bool{}
		for i, p := range paragraphs[sp.start:sp.end] {
			if quoted[sp.start+i] {
				continue
			}
			if n, ok := article(p.Text); ok {
				nums = append(nums, n)
			} else if headingCandidate(p.Text) {
				scope.FailedCheck = "invalid article heading"
			}
			if kind, number, ok := structure(p.Text); ok {
				key := kind + "-" + strings.ToLower(number)
				if structures[key] {
					scope.FailedCheck = "duplicate structure anchor " + key
				}
				structures[key] = true
			}
		}
		if sp.kind != "tail" {
			if failureIndex < len(quoteFailures) && quoteFailures[failureIndex] < sp.end {
				scope.FailedCheck = "unbalanced quotation"
			}
			if scope.FailedCheck == "" {
				scope.FailedCheck = sequence(nums)
			}
			scope.Accepted = scope.FailedCheck == ""
			if !scope.Accepted {
				r.Gaps = appendOnce(r.Gaps, "structure_uncertain")
			}
		}
		if sp.kind == "instrument" {
			instrumentNumber++
		}
		r.Scopes = append(r.Scopes, scope)
		prefix := ""
		if sp.kind == "instrument" {
			prefix = fmt.Sprintf("kem-theo-%d/", instrumentNumber)
		}
		if scope.Accepted {
			r.Provisions = append(r.Provisions, accepted(paragraphs[sp.start:sp.end], quoted[sp.start:sp.end], index, prefix, instrumentNumber)...)
		} else {
			r.Provisions = append(r.Provisions, neutral(paragraphs[sp.start:sp.end], quoted[sp.start:sp.end], index, sp.start)...)
		}
	}
	numberNeutral(r.Provisions)
	addContentGaps(&r, paragraphs)
	return r
}
func numberNeutral(provisions []Provision) {
	n := 0
	for i := range provisions {
		p := &provisions[i]
		if p.Kind != "other" {
			continue
		}
		n++
		p.Anchor = fmt.Sprintf("doan-%d", n)
		sum := sha256.Sum256([]byte(fmt.Sprintf("%d\n%s\n%s", p.ScopeIndex, p.Anchor, p.Text)))
		p.NodeID = fmt.Sprintf("tx:%x", sum[:8])
	}
}
func preamble(p []Paragraph, quoted []bool) []Provision {
	var out []Provision
	n := 0
	for start := 0; start < len(p); {
		if p[start].Table {
			start++
			continue
		}
		end := start + 1
		for end < len(p) && !p[end].Table {
			end++
		}
		for _, v := range neutral(p[start:end], quoted[start:end], -1, start) {
			if v.Kind == "other" {
				n++
				v.Anchor = fmt.Sprintf("doan-%d", n)
				sum := sha256.Sum256([]byte(fmt.Sprintf("%d\n%s\n%s", v.ScopeIndex, v.Anchor, v.Text)))
				v.NodeID = fmt.Sprintf("tx:%x", sum[:8])
			}
			out = append(out, v)
		}
		start = end
	}
	return out
}
func addContentGaps(r *Result, paragraphs []Paragraph) {
	indices := make(map[int]int, len(paragraphs))
	for i, p := range paragraphs {
		indices[p.Ordinal] = i
	}
	covered := make([]int, len(paragraphs)+1)
	for _, v := range r.Provisions {
		start, okStart := indices[v.StartOrdinal]
		end, okEnd := indices[v.EndOrdinal]
		if okStart && okEnd && start <= end {
			covered[start]++
			covered[end+1]--
		}
	}
	depth := 0
	for i, p := range paragraphs {
		depth += covered[i]
		if depth > 0 {
			n := utf8.RuneCountInString(p.Text)
			r.Counts.ProvisionCharacters += n
			if p.Table {
				r.Counts.TableCharacters += n
			}
		}
	}
	if r.Counts.TableCharacters*10 > r.Counts.ProvisionCharacters {
		r.Gaps = append(r.Gaps, "table_text")
	}
	if r.Counts.Images > 0 {
		r.Gaps = append(r.Gaps, "image_content")
	}
}
func appendOnce(s []string, v string) []string {
	for _, x := range s {
		if x == v {
			return s
		}
	}
	return append(s, v)
}
func scopes(p []Paragraph, quoted []bool) []span {
	first := -1
	for i := range p {
		if _, ok := article(p[i].Text); ok && !quoted[i] {
			first = i
			break
		}
	}
	if first < 0 {
		return nil
	}
	mainStart := first
	for i := 0; i < first; i++ {
		if _, _, ok := structure(p[i].Text); ok && !quoted[i] {
			mainStart = i
			break
		}
	}
	spans := []span{{start: mainStart, kind: "main"}}
	active := true
	seenArticle := false
	for i := first; i < len(p); i++ {
		s := p[i].Text
		if quoted[i] {
			continue
		}
		n, isArticle := article(s)
		inst := instrumentAt(p, i)
		if inst {
			found := false
			for j := i + 1; j < len(p); j++ {
				if quoted[j] {
					continue
				}
				if k, ok := article(p[j].Text); ok {
					found = k == "1"
					break
				}
				if instrumentAt(p, j) || signature(p[j].Text) || appendix(p[j].Text) {
					break
				}
			}
			if found {
				if active {
					spans[len(spans)-1].end = i
				}
				spans = append(spans, span{start: i, kind: "instrument"})
				active = true
				seenArticle = false
				continue
			}
		}
		if signature(s) && (!active || spans[len(spans)-1].kind != "tail") {
			if active {
				spans[len(spans)-1].end = i
				active = false
			}
			continue
		}
		if !active {
			if appendix(s) || inst || (isArticle && n == "1") {
				spans = append(spans, span{start: i, kind: "tail"})
				active = true
				seenArticle = isArticle
			}
			continue
		}
		if spans[len(spans)-1].kind != "tail" && (appendix(s) || inst || (isArticle && n == "1" && seenArticle)) {
			spans[len(spans)-1].end = i
			spans = append(spans, span{start: i, kind: "tail"})
			seenArticle = false
		}
		if isArticle {
			seenArticle = true
		}
	}
	if active {
		spans[len(spans)-1].end = len(p)
	}
	return spans
}
func makeProvision(p []Paragraph, kind, number, label, heading, anchor string, scope int) Provision {
	var lines []string
	for _, v := range p {
		lines = append(lines, v.Text)
	}
	text := strings.Join(lines, "\n")
	sum := sha256.Sum256([]byte(fmt.Sprintf("%d\n%s\n%s", scope, anchor, text)))
	return Provision{Kind: kind, Number: number, Label: label, Heading: heading, Anchor: anchor, ScopeIndex: scope, Text: text, StartOrdinal: p[0].Ordinal, EndOrdinal: p[len(p)-1].Ordinal, NodeID: fmt.Sprintf("tx:%x", sum[:8])}
}
func accepted(p []Paragraph, quoted []bool, scope int, prefix string, instrumentNumber int) []Provision {
	var out []Provision
	n := 0
	for start := 0; start < len(p); {
		end := start + 1
		for end < len(p) {
			if quoted[end] {
				end++
				continue
			}
			_, a := article(p[end].Text)
			_, _, h := structure(p[end].Text)
			if a || h {
				break
			}
			end++
		}
		if number, ok := article(p[start].Text); ok && !quoted[start] {
			label := "Điều " + number
			if prefix != "" {
				label = fmt.Sprintf("Điều %s (văn bản kèm theo %d)", number, instrumentNumber)
			}
			out = append(out, makeProvision(p[start:end], "dieu", number, label, p[start].Text, prefix+"dieu-"+number, scope))
		} else if kind, number, ok := structure(p[start].Text); ok && !quoted[start] {
			end = titleEnd(p, quoted, start)
			out = append(out, makeProvision(p[start:end], kind, number, p[start].Text, p[start].Text, prefix+kind+"-"+strings.ToLower(number), scope))
		} else {
			n++
			out = append(out, makeProvision(p[start:end], "other", "", "", "", fmt.Sprintf("%sdoan-%d", prefix, n), scope))
		}
		start = end
	}
	return out
}
func neutral(p []Paragraph, quoted []bool, scope, offset int) []Provision {
	_ = offset
	var out []Provision
	n := 0
	for start := 0; start < len(p); {
		end := start + 1
		for end < len(p) && end-start < 20 {
			_, a := article(p[end].Text)
			_, _, h := structure(p[end].Text)
			if !quoted[end] && (a || h || headingCandidate(p[end].Text) || outline(p[end].Text)) {
				break
			}
			end++
		}
		n++
		out = append(out, makeProvision(p[start:end], "other", "", "", "", fmt.Sprintf("doan-%d", n), scope))
		start = end
	}
	return out
}
func titleEnd(p []Paragraph, quoted []bool, start int) int {
	end := start + 1
	if bareStructure(p[start].Text) && end < len(p) && !quoted[end] && capitals(p[end].Text) {
		end++
	}
	return end
}
