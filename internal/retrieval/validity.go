package retrieval

import "time"

func dateValue(t *time.Time) *Date {
	if t == nil {
		return nil
	}
	return &Date{Year: t.Year(), Month: t.Month(), Day: t.Day()}
}

func (e *Engine) statusDate() Date {
	now := e.options.Now().In(time.FixedZone("Asia/Ho_Chi_Minh", 7*60*60))
	return Date{Year: now.Year(), Month: now.Month(), Day: now.Day()}
}

func servedStatusClass(stored string, from, to *Date, date Date) string {
	if stored != "in_force" && stored != "partial" {
		return stored
	}
	value := date.String()
	if to != nil && value >= to.String() {
		return "expired"
	}
	if from != nil && value < from.String() {
		return "not_yet"
	}
	return stored
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
	return Validity{Decisions: doc.Decisions, Unresolved: unresolved, StatusClass: servedStatusClass(doc.StatusClass, from, to, d), StatusBasis: doc.StatusBasis, Current: current, AsOf: arms.AsOf, EffectiveFrom: from, EffectiveTo: to, Basis: basis}, gaps
}
