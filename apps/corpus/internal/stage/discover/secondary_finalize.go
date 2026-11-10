package discover

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/source"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/stage/membership"
	"danny.vn/trachuan/apps/corpus/internal/store"
	"github.com/jackc/pgx/v5"
)

type secondaryObservation struct {
	key, hash string
	report    secondaryReport
}

func secondaryCeiling(observations []secondaryObservation, limit int) stage.Finalization {
	ids := map[string]bool{}
	refs := []string{}
	for _, o := range observations {
		for _, c := range o.report.Candidates {
			ids[c.ID] = true
		}
		if len(o.report.Candidates) > 0 {
			refs = append(refs, o.hash+":secondary.json")
		}
	}
	if len(ids) <= limit {
		return stage.Finalization{}
	}
	return stage.Finalization{Code: "secondary_candidate_ceiling", Reason: fmt.Sprintf("secondary candidates %d exceed ceiling %d", len(ids), limit), EvidenceRefs: refs}
}
func secondaryPriorState(s membership.SupportTuple, observations map[string]secondaryObservation) string {
	key, id, ok := strings.Cut(s.RuleValue, "|")
	if !ok {
		return "removed"
	}
	o, ok := observations[key]
	if !ok {
		return "removed"
	}
	if s.Evidence.QueryIdentity != nil && o.report.QueryIdentity != "" && *s.Evidence.QueryIdentity != o.report.QueryIdentity {
		return "removed"
	}
	if !o.report.Stable {
		return "retained"
	}
	for _, c := range o.report.Candidates {
		if c.ID == id {
			switch c.State {
			case "incomplete", "invalid", "ambiguous", "named_ambiguous", "not_attempted", "conflicting_detail":
				return "retained"
			}
			return "removed"
		}
	}
	return "removed"
}
func readSecondaryObservations(ctx context.Context, q *store.Queries, env stage.Env, r stage.Run, rules source.ChinhphuSources) ([]secondaryObservation, error) {
	seen := map[string]bool{}
	var observations []secondaryObservation
	for _, group := range []struct {
		prefix string
		values []string
	}{{"chinhphu:keyword:", rules.Keywords}, {"chinhphu:named:", rules.NamedNumbers}} {
		for _, value := range group.values {
			key := group.prefix + value
			if seen[key] {
				continue
			}
			seen[key] = true
			definition, err := secondaryItem(key, rules)
			if err != nil {
				return nil, err
			}
			row, err := q.GetRunItemArtifact(ctx, store.GetRunItemArtifactParams{RunID: r.ID, Stage: "discover", ItemKey: key})
			if errors.Is(err, pgx.ErrNoRows) {
				observations = append(observations, secondaryObservation{key: key, report: secondaryReport{Definition: definition, QueryIdentity: secondaryIdentity(definition)}})
				continue
			}
			if err != nil {
				return nil, fmt.Errorf("read secondary item %s: %w", key, err)
			}
			o := secondaryObservation{key: key, hash: row.Sha256, report: secondaryReport{Definition: definition, QueryIdentity: secondaryIdentity(definition)}}
			if row.State != "done" {
				observations = append(observations, o)
				continue
			}
			dir := env.Artifacts.Abs(row.Path)
			hash, err := artifact.Verify(dir)
			if err != nil {
				return nil, fmt.Errorf("verify secondary item: %w", err)
			}
			if hash != row.Sha256 {
				return nil, fmt.Errorf("secondary artifact %s hash mismatch", key)
			}
			if err = readJSON(dir, "secondary.json", &o.report); err != nil {
				return nil, err
			}
			if o.report.Version != 1 {
				return nil, fmt.Errorf("unsupported secondary artifact version %d", o.report.Version)
			}
			expected, err := secondaryItem(key, rules)
			if err != nil {
				return nil, err
			}
			if o.report.QueryIdentity != secondaryIdentity(expected) || o.report.Definition != expected {
				return nil, fmt.Errorf("secondary item %s identity mismatch", key)
			}
			observations = append(observations, o)
		}
	}
	return observations, nil
}

func priorEvidenceItem(s membership.SupportTuple) string {
	if s.RuleKind == "secondary_probe" {
		if key, _, ok := strings.Cut(s.RuleValue, "|"); ok {
			return key
		}
	}
	return "vbpl:central"
}
