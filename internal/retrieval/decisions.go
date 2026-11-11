package retrieval

import (
	"context"
	"time"
)

// LoadDecisions attaches ordered validity histories to the supplied returned hits.
// Callers of Shape may pass its Hits and NonCurrent slices after shaping.
func (e *Engine) LoadDecisions(ctx context.Context, groups ...[]Hit) error {
	var keys []string
	seen := map[string]bool{}
	for _, hits := range groups {
		for _, hit := range hits {
			if !seen[hit.Identity.DocumentKey] {
				keys = append(keys, hit.Identity.DocumentKey)
				seen[hit.Identity.DocumentKey] = true
			}
		}
	}
	if len(keys) == 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, e.deadline())
	defer cancel()
	rows, err := e.db.Query(ctx, `SELECT d.key,v.ordinal,v.layer,v.rule_version,v.before,v.after,v.actor_key,v.effect_date,v.relation_id,v.anchor,v.quote,v.artifact_sha256 FROM document d JOIN validity_decision v ON v.document_id=d.id WHERE d.key=ANY($1::text[]) ORDER BY d.key,v.ordinal`, keys)
	if err != nil {
		return databaseFailure("query validity decisions failed", err)
	}
	defer rows.Close()
	decisions := map[string][]ValidityDecision{}
	for rows.Next() {
		var key string
		var v ValidityDecision
		var effect *time.Time
		if err = rows.Scan(&key, &v.Ordinal, &v.Layer, &v.RuleVersion, &v.Before, &v.After, &v.ActorKey, &effect, &v.RelationID, &v.Anchor, &v.Quote, &v.ArtifactSHA256); err != nil {
			return databaseFailure("read validity decision failed", err)
		}
		v.EffectDate = dateValue(effect)
		decisions[key] = append(decisions[key], v)
	}
	if err = rows.Err(); err != nil {
		return databaseFailure("read validity decisions failed", err)
	}
	for _, hits := range groups {
		for i := range hits {
			hits[i].Stamp.Decisions = decisions[hits[i].Identity.DocumentKey]
			if hits[i].Stamp.Decisions == nil {
				hits[i].Stamp.Decisions = []ValidityDecision{}
			}
		}
	}
	return nil
}
