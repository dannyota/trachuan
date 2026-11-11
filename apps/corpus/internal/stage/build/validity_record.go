package build

func addValidityCounts(counts map[string]int64, validity validityCounts) {
	for _, basis := range []string{"source_status", "derived_from_consolidated", "derived_from_text", "manual_correction"} {
		counts["documents_by_status_basis_"+basis] = int64(validity.basis[basis])
	}
	counts["rule_a_corrections"] = int64(validity.ruleA)
	counts["rule_b_events"] = int64(validity.ruleB)
	counts["rule_c_conflicts"] = int64(validity.ruleC)
	counts["validity_review_replaces_unverified"] = 0
	for kind, n := range validity.review {
		counts["validity_review_"+kind] = int64(n)
	}
}
