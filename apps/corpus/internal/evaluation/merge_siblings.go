package evaluation

import (
	"context"
	"fmt"

	"danny.vn/trachuan/internal/retrieval"
)

func hydrateSiblingMerge(ctx context.Context, engine Engine, arms retrieval.Arms) (*retrieval.SiblingMerge, error) {
	hydrator, ok := engine.(interface {
		HydrateSiblingMerge(context.Context, retrieval.Arms) (*retrieval.SiblingMerge, error)
	})
	if !ok {
		return nil, fmt.Errorf("evaluation engine does not support sibling merge")
	}
	merge, err := hydrator.HydrateSiblingMerge(ctx, arms)
	if err != nil {
		return nil, fmt.Errorf("hydrate sibling merge: %w", err)
	}
	if merge == nil {
		return nil, fmt.Errorf("missing sibling merge metadata")
	}
	return merge, nil
}

func siblingGroupsMatch(merge *retrieval.SiblingMerge, arms retrieval.Arms, recipe retrieval.Recipe, limit int, mode retrieval.ArmMode, cohort string, expected Expectation, entry MapEntry, includeComplement bool) (bool, error) {
	hits, err := merge.ShapeGroups(arms, recipe, limit, mode)
	if err != nil {
		return false, fmt.Errorf("shape diagnostic sibling groups: %w", err)
	}
	if includeComplement {
		arms.Primary = arms.NonCurrent
		other, err := merge.ShapeGroups(arms, recipe, limit, mode)
		if err != nil {
			return false, fmt.Errorf("shape diagnostic non-current sibling groups: %w", err)
		}
		hits = append(hits, other...)
	}
	return diagnosticRank(cohort, expected, entry, hits) > 0, nil
}
