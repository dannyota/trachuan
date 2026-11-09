package stage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"danny.vn/trachuan/apps/corpus/internal/store"
	"github.com/jackc/pgx/v5"
)

// Finalizer publishes stage results in the close transaction without committing,
// making network requests, or changing immutable artifacts. It must be
// idempotent for identical inputs and reject changed input after finalization.
type Finalizer interface {
	Finalize(context.Context, pgx.Tx, Run) (Finalization, error)
}

// Finalization describes success when Code is empty, or a durable domain failure.
type Finalization struct {
	Code, Reason, ItemKey string
	EvidenceRefs          []string
}

// failureIdentity hashes the JSON tuple of code, stage, item key and reason, so
// the same failure is idempotent without ambiguous separators.
func failureIdentity(stage string, f Finalization) (string, error) {
	data, err := json.Marshal([]string{f.Code, stage, f.ItemKey, f.Reason})
	if err != nil {
		return "", fmt.Errorf("encode failure identity: %w", err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func recordFinalization(ctx context.Context, q *store.Queries, in StageInput, f Finalization) error {
	if f.Reason == "" {
		return fmt.Errorf("finalization failure %q has no reason", f.Code)
	}
	identity, err := failureIdentity(in.Stage, f)
	if err != nil {
		return err
	}
	refs := f.EvidenceRefs
	if refs == nil {
		refs = []string{}
	}
	evidence, err := json.Marshal(refs)
	if err != nil {
		return fmt.Errorf("encode failure evidence: %w", err)
	}
	err = q.InsertRunFailure(ctx, store.InsertRunFailureParams{RunID: in.RunID, FailureIdentity: identity, Code: f.Code, Stage: in.Stage, ItemKey: f.ItemKey, Reason: f.Reason, EvidenceRefs: evidence})
	if err != nil {
		return fmt.Errorf("insert run failure: %w", err)
	}
	if err = q.SetRunState(ctx, store.SetRunStateParams{ID: in.RunID, State: "failed", Message: &f.Reason}); err != nil {
		return fmt.Errorf("mark run failed: %w", err)
	}
	return nil
}

// BlockingFailureCodes lists non-acceptable run failure codes.
func BlockingFailureCodes() []string { return []string{"scope_changed"} }
