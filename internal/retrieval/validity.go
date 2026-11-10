package retrieval

import "time"

func dateValue(t *time.Time) *Date {
	if t == nil {
		return nil
	}
	return &Date{Year: t.Year(), Month: t.Month(), Day: t.Day()}
}

func stamp(doc Document, arms Arms) (Validity, []Gap) {
	d := arms.Date
	if arms.AsOf != nil {
		d = *arms.AsOf
	}
	value := d.String()
	from, to := dateValue(doc.EffectiveFrom), dateValue(doc.EffectiveTo)
	eligible := (from == nil || from.String() <= value) && (to == nil || value < to.String())
	basis := "source_status"
	current := eligible && !doc.ValidityConflict && doc.IsCurrent && (doc.StatusClass == "in_force" || doc.StatusClass == "partial")
	if arms.AsOf != nil {
		basis = "document_dates"
		current = eligible && from != nil
	}
	unresolved := arms.AsOf == nil && doc.ValidityConflict
	var gaps []Gap
	if unresolved {
		gaps = append(gaps, Gap{Kind: "validity_conflict", Message: "Document validity is unresolved."})
	}
	for _, kind := range doc.DerivationGaps {
		gaps = append(gaps, Gap{Kind: kind, Message: "Consolidated document status could not be fully derived."})
	}
	if from == nil {
		gaps = append(gaps, Gap{Kind: "missing_effective_date", Message: "Effective start date is missing."})
	}
	return Validity{Decisions: doc.Decisions, Unresolved: unresolved, StatusClass: doc.StatusClass, StatusBasis: doc.StatusBasis, Current: current, AsOf: arms.AsOf, EffectiveFrom: from, EffectiveTo: to, Basis: basis}, gaps
}
