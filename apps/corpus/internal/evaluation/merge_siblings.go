package evaluation

import (
	"fmt"

	"danny.vn/trachuan/internal/retrieval"
)

func siblingGroupsMatch(arms retrieval.Arms, recipe retrieval.Recipe, limit int, mode retrieval.ArmMode, cohort string, expected Expectation, entry MapEntry, includeComplement bool) (bool, error) {
	hits, err := retrieval.ShapeGroups(arms, recipe, limit, mode)
	if err != nil {
		return false, fmt.Errorf("shape diagnostic sibling groups: %w", err)
	}
	if includeComplement {
		arms.Primary = arms.NonCurrent
		other, err := retrieval.ShapeGroups(arms, recipe, limit, mode)
		if err != nil {
			return false, fmt.Errorf("shape diagnostic non-current sibling groups: %w", err)
		}
		hits = append(hits, other...)
	}
	return diagnosticRank(cohort, expected, entry, hits) > 0, nil
}
