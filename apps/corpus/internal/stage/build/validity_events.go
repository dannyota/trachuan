package build

import (
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"danny.vn/trachuan/apps/corpus/internal/source"
	"danny.vn/trachuan/apps/corpus/internal/validity"
	"danny.vn/trachuan/internal/retrieval"
)

type validityEvent struct {
	actor, target, date string
	item                validity.Item
}

func verifiedIssuer(item validity.Item, d validityDocument) bool {
	if item.Issuer != "" && source.NormalizeIssuer(item.Issuer) == source.NormalizeIssuer(d.issuer) {
		return true
	}
	number := retrieval.NormalizeNumber(d.number)
	code := ""
	switch source.NormalizeIssuer(d.issuer) {
	case "CHÍNH PHỦ":
		code = "ND-CP"
	case "QUỐC HỘI":
		if regexp.MustCompile(`^[0-9]+/[0-9]{4}/QH[0-9]+$`).MatchString(number) {
			return true
		}
	case "NGÂN HÀNG NHÀ NƯỚC VIỆT NAM":
		code = "TT-NHNN"
	}
	return code != "" && regexp.MustCompile(`^[0-9]+/[0-9]{4}/`+regexp.QuoteMeta(code)+`$`).MatchString(number)
}
func (e *validityEngine) events() []validityEvent {
	var events []validityEvent
	for _, key := range e.keys {
		d := e.docs[key]
		date := e.operative[key]
		if d.consolidated || d.article == nil || !d.article.Usable {
			continue
		}
		for _, item := range d.article.Items {
			if item.Scope != "whole" {
				continue
			}
			if date == "" {
				e.reviewItem("rule_b_skipped", d, item, "missing_operative_date")
				continue
			}
			var found []validityDocument
			issue := parsedDate(item.IssueDate)
			if issue == "" {
				e.reviewItem("rule_b_skipped", d, item, "missing_issue_date")
				continue
			}
			for _, targetKey := range e.keys {
				target := e.docs[targetKey]
				if target.numberNorm == retrieval.NormalizeNumber(item.Number) && strings.EqualFold(strings.TrimSpace(target.docType), strings.TrimSpace(item.InstrumentType)) && target.issue == issue {
					found = append(found, target)
				}
			}
			if len(found) != 1 {
				reason := "not_found"
				if len(found) > 1 {
					reason = "ambiguous"
				}
				e.reviewItem("rule_b_skipped", d, item, reason)
				continue
			}
			target := found[0]
			if !verifiedIssuer(item, target) {
				e.reviewItemTarget("rule_b_skipped", d, target, item, "issuer_unverified")
				continue
			}
			targetFrom := e.operative[target.key]
			if targetFrom == "" {
				targetFrom = target.from
			}
			if targetFrom == "" || date <= targetFrom {
				e.reviewItemTarget("rule_b_skipped", d, target, item, "invalid_timing")
				continue
			}
			if e.corrections[target.key].Hold || target.consolidated {
				e.reviewItemTarget("rule_b_skipped", d, target, item, "held_or_consolidated_target")
				continue
			}
			events = append(events, validityEvent{actor: key, target: target.key, date: date, item: item})
			e.counts.ruleB++
		}
	}
	sort.Slice(events, func(i, j int) bool {
		a, b := events[i], events[j]
		if a.target != b.target {
			return a.target < b.target
		}
		if a.date != b.date {
			return a.date < b.date
		}
		if a.actor != b.actor {
			return a.actor < b.actor
		}
		return a.item.Text < b.item.Text
	})
	return events
}
func partialMentions(r *validity.Result, number string) bool {
	if r == nil {
		return false
	}
	candidate := regexp.MustCompile(`[0-9]{1,4}[\s\p{Z}]*/[\s\p{Z}]*(?:[0-9]{4}[\s\p{Z}]*/[\s\p{Z}]*)?[\p{Latin}][\p{Latin}0-9]*(?:[\s\p{Z}]*-[\s\p{Z}]*[\p{Latin}0-9]+)*`)
	for _, item := range r.Items {
		if item.Scope != "partial" {
			continue
		}
		for _, loc := range candidate.FindAllStringIndex(item.Text, -1) {
			if loc[0] > 0 {
				r, _ := utf8.DecodeLastRuneInString(item.Text[:loc[0]])
				if unicode.IsLetter(r) || unicode.IsDigit(r) {
					continue
				}
			}
			if loc[1] < len(item.Text) {
				r, _ := utf8.DecodeRuneInString(item.Text[loc[1]:])
				if unicode.IsLetter(r) || unicode.IsDigit(r) {
					continue
				}
			}
			if retrieval.NormalizeNumber(item.Text[loc[0]:loc[1]]) == number {
				return true
			}
		}
	}
	return false
}
