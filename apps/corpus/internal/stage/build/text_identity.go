package build

import "danny.vn/trachuan/apps/corpus/internal/source"

func TextTargetIdentity(issuerCode string, statedIssueDate *string, targetIssuer, targetIssueDate string, issuers map[string]string) bool {
	expected := source.NormalizeIssuer(issuers[issuerCode])
	if expected == "" || source.NormalizeIssuer(targetIssuer) != expected {
		return false
	}
	if statedIssueDate != nil {
		stated, err := ParseDate(*statedIssueDate)
		if err != nil || stated == nil {
			return false
		}
		actual, err := ParseDate(targetIssueDate)
		if err != nil || actual == nil || !stated.Equal(*actual) {
			return false
		}
	}
	return true
}
