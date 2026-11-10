package build

import "danny.vn/trachuan/apps/corpus/internal/source"

type validityValues struct {
	Class    string  `json:"status_class"`
	Current  bool    `json:"is_current"`
	Basis    string  `json:"status_basis"`
	From     *string `json:"eff_from"`
	To       *string `json:"eff_to"`
	Conflict bool    `json:"validity_conflict"`
}

func applyCorrection(before validityValues, correction source.Correction, automatic bool) (validityValues, string) {
	after := before
	changed := false
	if value := correction.Set.StatusClass; value != nil {
		changed = changed || after.Class != *value
		after.Class = *value
		after.Current = *value == "in_force" || *value == "partial"
	}
	if value := correction.Set.EffFrom; value != nil {
		changed = changed || after.From == nil || *after.From != *value
		after.From = value
	}
	if value := correction.Set.EffTo; value != nil {
		changed = changed || after.To == nil || *after.To != *value
		after.To = value
	}
	if value := correction.Eligible; value != nil {
		changed = changed || after.Conflict == *value
		after.Conflict = !*value
	}
	if correction.Set.StatusClass != nil || correction.Set.EffFrom != nil || correction.Set.EffTo != nil || correction.Eligible != nil {
		after.Basis = "manual_correction"
	}
	if !changed && !correction.Hold {
		return after, "correction_redundant"
	}
	if changed && automatic {
		return after, "correction_conflict"
	}
	return after, ""
}
