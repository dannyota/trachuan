package build

import (
	"fmt"

	"danny.vn/trachuan/apps/corpus/internal/source"
)

func (e *validityEngine) apply(events []validityEvent) error {
	for _, event := range events {
		o := e.outputs[event.target]
		if o.values.To != nil && *o.values.To <= event.date {
			continue
		}
		before := o.values
		o.values.To = ptrDate(event.date)
		o.values.Basis = "derived_from_text"
		actor := e.docs[event.actor]
		agreement := int64(0)
		for _, rel := range e.relations {
			if rel.source == event.actor && rel.target == event.target && rel.kind == event.item.Verb {
				agreement = rel.id
				break
			}
		}
		o.history = append(o.history, validityDecision{layer: "B", version: "validity/2", before: before, after: o.values, actor: event.actor, date: event.date, relationID: agreement, anchor: actor.anchor, quote: truncateQuote(event.item.Sentence), artifact: actor.extractHash})
		e.outputs[event.target] = o
	}
	for _, rel := range e.relations {
		actor, ok := e.docs[rel.source]
		if !ok {
			continue
		}
		target, ok := e.docs[rel.target]
		if !ok || actor.consolidated || target.consolidated {
			continue
		}
		if rel.kind != "replaces" && rel.kind != "repeals" {
			continue
		}
		a := e.outputs[actor.key].values
		if correction, ok := e.corrections[actor.key]; ok {
			a, _ = applyCorrection(a, correction, false)
		}
		t := e.outputs[target.key].values
		date := e.operative[actor.key]
		if !a.Current || a.From == nil || *a.From > e.asOf || (a.To != nil && *a.To <= e.asOf) || date == "" || date > e.asOf || !t.Current {
			continue
		}
		if rel.kind == "repeals" {
			e.review("repeals_current", actor, target.key, target.number, "")
			continue
		}
		if target.sourceClass == "partial" {
			e.review("partial_target", actor, target.key, target.number, "")
			continue
		}
		if e.corrections[target.key].Hold {
			continue
		}
		if target.sourceClass != "in_force" {
			continue
		}
		if actor.article == nil || !actor.article.Usable {
			e.reviewReplacesUnverified(actor, target, rel.id)
			continue
		}
		if articleMentions(actor.article, target.numberNorm) {
			continue
		}
		before := t
		t.Conflict = true
		o := e.outputs[target.key]
		o.values = t
		o.history = append(o.history, validityDecision{layer: "C", version: "validity/2", before: before, after: t, actor: actor.key, date: date, relationID: rel.id, anchor: actor.anchor, quote: truncateQuote(actor.quote), artifact: actor.extractHash})
		e.outputs[target.key] = o
		e.counts.ruleC++
		e.review("rule_c_conflict", actor, target.key, target.number, "")
	}
	for _, key := range e.keys {
		doc := e.docs[key]
		o := e.outputs[key]
		c, has := e.corrections[key]
		if has {
			if c.Evidence.URL != "" {
				e.review("correction_external", doc, "", "", c.Evidence.URL)
				r := &e.reviews[len(e.reviews)-1]
				r.sentences = []string{c.Evidence.Quote}
				r.proposed = &c
			}
			before := o.values
			automatic := false
			if len(o.history) > 0 {
				automatic = correctionFieldsChanged(o.history[0].before, before, c)
			}
			after, report := applyCorrection(before, c, automatic)
			o.values = after
			if report != "" {
				e.review(report, doc, "", "", "")
			}
			if c.Set.StatusClass != nil || c.Set.EffFrom != nil || c.Set.EffTo != nil || c.Eligible != nil {
				artifact := doc.extractHash
				anchor := c.Evidence.Anchor
				if c.Evidence.URL != "" {
					anchor = "external:" + c.Evidence.URL
					artifact = ""
				} else if c.Evidence.Key != "" {
					artifact = e.docs[c.Evidence.Key].extractHash
				}
				o.history = append(o.history, validityDecision{layer: "manual", version: "validity/2", before: before, after: after, actor: c.Evidence.Key, anchor: anchor, quote: truncateQuote(c.Evidence.Quote), artifact: artifact})
			}
		}
		if o.values.From != nil && o.values.To != nil && *o.values.To < *o.values.From {
			return fmt.Errorf("validity %s: invalid interval", key)
		}
		if len(o.history) > 0 {
			base := o.history[0].before
			o.history = append([]validityDecision{{layer: "source", version: "validity/2", before: base, after: base}}, o.history...)
		}
		if len(o.history) > 8 {
			return fmt.Errorf("validity %s: decision history exceeds 8", key)
		}
		e.outputs[key] = o
	}
	return nil
}

func correctionFieldsChanged(base, after validityValues, c source.Correction) bool {
	if c.Set.StatusClass != nil && base.Class != after.Class && *c.Set.StatusClass != after.Class {
		return true
	}
	if c.Set.EffFrom != nil && !sameDate(base.From, after.From) && !sameDate(c.Set.EffFrom, after.From) {
		return true
	}
	if c.Set.EffTo != nil && !sameDate(base.To, after.To) && !sameDate(c.Set.EffTo, after.To) {
		return true
	}
	return c.Eligible != nil && base.Conflict != after.Conflict && *c.Eligible == after.Conflict
}
func sameDate(a, b *string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}
