package build

import (
	"danny.vn/trachuan/apps/corpus/internal/source"
	"danny.vn/trachuan/apps/corpus/internal/validity"
)

func (e *validityEngine) review(kind string, d validityDocument, targetKey, targetNumber, detail string) {
	r := validityReview{kind: kind, key: d.key, number: d.number, targetKey: targetKey, targetNumber: targetNumber, detail: detail, statusCode: d.statusCode, statusName: d.statusName, rawFrom: d.rawFrom, rawTo: d.rawTo, extractHash: d.extractHash, fetchHash: d.fetchHash}
	if target, ok := e.docs[targetKey]; ok {
		r.targetStatusCode, r.targetStatusName = target.statusCode, target.statusName
		r.targetRawFrom, r.targetRawTo = target.rawFrom, target.rawTo
		r.targetFetchHash, r.targetExtractHash = target.fetchHash, target.extractHash
	}
	if d.quote != "" {
		r.sentences = append(r.sentences, d.quote)
	}
	if d.article != nil {
		for _, reason := range d.article.Reasons {
			if reason.Sentence != "" {
				r.sentences = append(r.sentences, reason.Sentence)
			}
		}
		for _, item := range d.article.Items {
			if item.Sentence != "" {
				r.sentences = append(r.sentences, item.Sentence)
			}
		}
		if date := parsedDate(d.article.Commencement); date != "" {
			status := "in_force"
			r.proposed = &source.Correction{Key: d.key, Number: d.number, Set: source.CorrectionSet{StatusClass: &status, EffFrom: &date}, Evidence: source.CorrectionEvidence{Anchor: d.anchor, Quote: d.quote}}
		}
	}
	e.reviews = append(e.reviews, r)
}
func (e *validityEngine) reviewItem(kind string, d validityDocument, item validity.Item, detail string) {
	e.review(kind, d, "", item.Number, detail)
	r := &e.reviews[len(e.reviews)-1]
	r.sentences = []string{item.Sentence}
}

func (e *validityEngine) reviewItemTarget(kind string, d, target validityDocument, item validity.Item, detail string) {
	e.review(kind, d, target.key, target.number, detail)
	e.reviews[len(e.reviews)-1].sentences = []string{item.Sentence}
}

func (e *validityEngine) reviewReplacesUnverified(actor, target validityDocument, relationID int64) {
	e.review("replaces_unverified", actor, target.key, target.number, "")
	r := &e.reviews[len(e.reviews)-1]
	r.relationID = relationID
	r.sentences = nil
	if actor.quote != "" {
		r.sentences = []string{truncateQuote(actor.quote)}
	}
	r.proposed = &source.Correction{Key: target.key, Number: target.number}
}
