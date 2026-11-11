package build

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/source"
	"danny.vn/trachuan/apps/corpus/internal/validity"
	"danny.vn/trachuan/internal/retrieval"
)

type validityEngine struct {
	docs        map[string]validityDocument
	keys        []string
	relations   []validityRelation
	corrections map[string]source.Correction
	operative   map[string]string
	outputs     map[string]validityOutputDocument
	reviews     []validityReview
	counts      validityCounts
	asOf        string
}

func runValidity(docs []validityDocument, relations []validityRelation, corrections map[string]source.Correction, asOf string) (validityResult, error) {
	if _, err := time.Parse("2006-01-02", asOf); err != nil {
		return validityResult{}, fmt.Errorf("invalid build date: %w", err)
	}
	e := validityEngine{docs: map[string]validityDocument{}, corrections: corrections, operative: map[string]string{}, outputs: map[string]validityOutputDocument{}, asOf: asOf, counts: validityCounts{basis: map[string]int{}, review: map[string]int{}}}
	for _, d := range docs {
		if d.key == "" {
			return validityResult{}, fmt.Errorf("empty validity document key")
		}
		if _, ok := e.docs[d.key]; ok {
			return validityResult{}, fmt.Errorf("duplicate validity document %s", d.key)
		}
		if d.numberNorm == "" {
			d.numberNorm = retrieval.NormalizeNumber(d.number)
		}
		if d.textSource != "" && d.textSource != "source_tree" && d.article != nil && !d.consolidated {
			e.review("parsed_text_article", d, "", "", "")
			if e.reviews[len(e.reviews)-1].proposed == nil {
				e.reviews[len(e.reviews)-1].proposed = &source.Correction{Key: d.key, Number: d.number, Evidence: source.CorrectionEvidence{Anchor: d.anchor, Quote: d.quote}}
			}
			d.article = nil
		}
		e.docs[d.key] = d
		e.keys = append(e.keys, d.key)
	}
	sort.Strings(e.keys)
	e.relations = append([]validityRelation(nil), relations...)
	sort.Slice(e.relations, func(i, j int) bool {
		a, b := e.relations[i], e.relations[j]
		if a.source != b.source {
			return a.source < b.source
		}
		if a.target != b.target {
			return a.target < b.target
		}
		if a.kind != b.kind {
			return a.kind < b.kind
		}
		return a.id < b.id
	})
	for _, key := range e.keys {
		e.initialize(e.docs[key])
		e.commencement(e.docs[key])
	}
	events := e.events()
	if err := e.apply(events); err != nil {
		return validityResult{}, err
	}
	for _, key := range e.keys {
		d := e.outputs[key]
		e.counts.basis[d.values.Basis]++
		if d.values.Class == "unknown" {
			e.review("unknown", e.docs[key], "", "", "")
		}
	}
	for _, review := range e.reviews {
		e.counts.review[review.kind]++
	}
	return validityResult{documents: e.outputs, reviews: e.reviews, counts: e.counts}, nil
}

func ptrDate(s string) *string {
	if s == "" {
		return nil
	}
	x := s
	return &x
}
func parsedDate(d validity.Date) string {
	if d.Year == 0 {
		return ""
	}
	return fmt.Sprintf("%04d-%02d-%02d", d.Year, d.Month, d.Day)
}
func (e *validityEngine) initialize(d validityDocument) {
	basis := d.basis
	if basis == "" {
		basis = "source_status"
	}
	class := d.sourceClass
	if d.initialClass != "" {
		class = d.initialClass
	}
	v := validityValues{Class: class, Current: class == "in_force" || class == "partial", Basis: basis, From: ptrDate(d.from), To: ptrDate(d.to)}
	e.outputs[d.key] = validityOutputDocument{values: v}
}
func (e *validityEngine) incoming(key string) bool {
	for _, r := range e.relations {
		if r.target != key {
			continue
		}
		if _, ok := e.docs[r.source]; !ok {
			continue
		}
		switch r.kind {
		case "amends_supplements", "suspends", "replaces", "repeals":
			return true
		}
	}
	return false
}
func (e *validityEngine) commencement(d validityDocument) {
	if d.consolidated {
		return
	}
	correction := e.corrections[d.key]
	if d.article != nil && !d.article.Usable {
		e.review("unresolved_article", d, "", "", "")
	}
	if correction.Set.EffFrom != nil {
		e.operative[d.key] = *correction.Set.EffFrom
		return
	}
	candidate := d.sourceClass == "not_yet" || (strings.TrimSpace(d.statusCode) == "" && strings.TrimSpace(d.statusName) == "")
	if strings.TrimSpace(d.statusCode) == "" && strings.TrimSpace(d.statusName) != "" && d.sourceClass == "unknown" {
		e.review("rule_a_status_name", d, "", "", "")
	}
	date := ""
	if d.article != nil && d.article.Usable {
		date = parsedDate(d.article.Commencement)
	}
	if candidate && !correction.Hold && date != "" {
		blocked := ""
		switch {
		case d.issue == "" || date < d.issue:
			blocked = "issue_date"
		case d.rawTo != "" || d.to != "":
			blocked = "end_date"
		case d.dateGap:
			blocked = "date_gap"
		case e.incoming(d.key):
			blocked = "relation"
		}
		if blocked == "" {
			o := e.outputs[d.key]
			before := o.values
			o.values.Class = "in_force"
			o.values.Current = true
			o.values.Basis = "derived_from_text"
			o.values.From = ptrDate(date)
			o.history = append(o.history, validityDecision{layer: "A", version: "validity/2", before: before, after: o.values, anchor: d.anchor, quote: truncateQuote(d.quote), artifact: d.extractHash})
			e.outputs[d.key] = o
			e.counts.ruleA++
			e.operative[d.key] = date
			return
		}
		e.review("rule_a_blocked", d, "", "", blocked)
	}
	if date != "" && d.from != "" && date != d.from {
		e.review("date_conflict", d, "", "", date)
		return
	}
	if date != "" && d.from == "" {
		e.review("date_conflict", d, "", "", date)
		return
	}
	e.operative[d.key] = d.from
}
func truncateQuote(s string) string {
	r := []rune(s)
	if len(r) > 400 {
		return string(r[:400])
	}
	return s
}
