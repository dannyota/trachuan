package evaluation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"danny.vn/trachuan/internal/retrieval"
)

func sameValidityOverrides(a, b ValidityOverrides) bool {
	return sameOptional(a.HistoricalPolicy, b.HistoricalPolicy) && sameOptional(a.NonCurrentTail, b.NonCurrentTail) && a.NotYetEligible == b.NotYetEligible
}

func inForceForCase(c Case, policy ValidityOverrides) bool {
	if policy.HistoricalPolicy != nil && *policy.HistoricalPolicy == "agent" {
		for _, tag := range c.Tags {
			if tag == "historical" {
				return false
			}
		}
	}
	return true
}

func validityEligibilityHash(date retrieval.Date, force bool, policy ValidityOverrides, arms retrieval.Arms) string {
	hash := eligibilityHash(date)
	if !force || policy.NotYetEligible || (policy.NonCurrentTail != nil && *policy.NonCurrentTail > 0) {
		sum := sha256.Sum256([]byte(fmt.Sprintf("%s/%t/%t/%t", hash, force, policy.NotYetEligible, arms.IncludeNonCurrent)))
		return hex.EncodeToString(sum[:])
	}
	return hash
}

func sameOptional[T comparable](a, b *T) bool {
	return (a == nil && b == nil) || (a != nil && b != nil && *a == *b)
}

func validateValidityOverrides(v ValidityOverrides) error {
	if v.HistoricalPolicy != nil && *v.HistoricalPolicy != "agent" {
		return fmt.Errorf("invalid historical policy")
	}
	if v.NonCurrentTail != nil && (*v.NonCurrentTail < 0 || *v.NonCurrentTail > 5) {
		return fmt.Errorf("invalid non-current tail")
	}
	return nil
}

func collectEvaluation(ctx context.Context, engine Engine, prepared retrieval.Prepared, depths retrieval.Depths, force bool, policy ValidityOverrides, lexical *retrieval.LexicalArms) (retrieval.Arms, error) {
	if force && policy.NonCurrentTail != nil && *policy.NonCurrentTail > 0 {
		include := false
		prepared.Request.InForceOnly = &include
	}
	if !policy.NotYetEligible {
		if lexical != nil {
			if collector, ok := engine.(interface {
				CollectWithLexical(context.Context, retrieval.Prepared, retrieval.Depths, retrieval.LexicalArms) (retrieval.Arms, error)
			}); ok {
				return collector.CollectWithLexical(ctx, prepared, depths, *lexical)
			}
		}
		return engine.Collect(ctx, prepared, depths)
	}
	collector, ok := engine.(interface {
		CollectWithNotYetEligible(context.Context, retrieval.Prepared, retrieval.Depths) (retrieval.Arms, error)
	})
	if !ok {
		return retrieval.Arms{}, fmt.Errorf("evaluation engine does not support not-yet eligibility")
	}
	return collector.CollectWithNotYetEligible(ctx, prepared, depths)
}
