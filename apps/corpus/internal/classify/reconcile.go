package classify

import (
	"context"
	"fmt"
	"strings"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/batchapi"
	"github.com/anthropics/anthropic-sdk-go/v2"
)

type batchLister interface {
	List(context.Context, time.Time, time.Time) ([]anthropic.MessageBatch, error)
}

type reconciledBatch struct {
	batch   anthropic.MessageBatch
	results []batchapi.Result
}

func (r *Runner) Reconcile(ctx context.Context, id string, abandon bool) error {
	if !validID(id) {
		return fmt.Errorf("invalid submission id")
	}
	if r.Now == nil {
		r.Now = time.Now
	}
	target, err := r.transition(ctx, id, func(s *Submission) (bool, error) {
		if s.State != "uncertain" && s.State != "abandoned" {
			return false, fmt.Errorf("only uncertain or abandoned submissions can be reconciled")
		}
		return false, nil
	})
	if err != nil {
		return err
	}
	if abandon && target.State == "abandoned" {
		return nil
	}
	lister, ok := r.Client.(batchLister)
	if !ok {
		return fmt.Errorf("provider batch listing is unavailable")
	}
	before := r.Now().Add(time.Minute)
	if minimum := target.CreatedAt.Add(10 * time.Minute); before.Before(minimum) {
		before = minimum
	}
	batches, err := lister.List(ctx, target.CreatedAt.Add(-time.Minute), before)
	if err != nil {
		return providerError(err)
	}
	var found *reconciledBatch
	allEnded := true
	for _, batch := range batches {
		if batch.ProcessingStatus != "ended" {
			allEnded = false
			continue
		}
		results, resultErr := r.Client.Results(ctx, batch.ID)
		if resultErr != nil {
			return providerError(resultErr)
		}
		for _, result := range results {
			if !strings.HasPrefix(result.CustomID, id+"-") {
				continue
			}
			if found != nil && found.batch.ID != batch.ID {
				return fmt.Errorf("multiple batches match submission; operator review required")
			}
			found = &reconciledBatch{batch: batch, results: results}
			break
		}
	}
	if found == nil {
		if !abandon {
			return fmt.Errorf("no ended provider batch matches submission")
		}
		if !allEnded || r.Now().Before(target.CreatedAt.Add(creatorQuietWindow)) {
			return fmt.Errorf("cannot abandon before the 24-hour provider window plus ten-minute margin or while candidate batches remain active")
		}
		creator, lockErr := r.lockCreator(id)
		if lockErr != nil {
			return lockErr
		}
		defer func() { _ = creator.close() }()
		quietSince, quietErr := r.creatorQuietSince(target)
		if quietErr != nil {
			return quietErr
		}
		if r.Now().Before(quietSince.Add(creatorQuietWindow)) {
			return fmt.Errorf("cannot abandon before the creator has been quiet for 24 hours plus ten minutes")
		}
	}
	_, err = r.transition(ctx, id, func(latest *Submission) (bool, error) {
		if !sameReconciliationTarget(target, *latest) {
			return false, fmt.Errorf("submission changed during reconciliation; retry from its current state")
		}
		if found == nil {
			if writeErr := r.writeAbandonment(latest); writeErr != nil {
				return false, writeErr
			}
			latest.State = "abandoned"
			return true, nil
		}
		if !found.batch.CreatedAt.IsZero() {
			latest.SubmittedAt = &found.batch.CreatedAt
		}
		if writeErr := r.recordBatch(latest, found.batch.ID); writeErr != nil {
			return false, writeErr
		}
		if target.State == "abandoned" {
			if applyErr := apply(latest, found.results); applyErr != nil {
				return false, applyErr
			}
			latest.State = "abandoned"
		}
		return true, nil
	})
	return err
}

func sameReconciliationTarget(a, b Submission) bool {
	return a.State == b.State && a.BatchID == b.BatchID && a.ItemsHash == b.ItemsHash && a.ScreenHash == b.ScreenHash && a.Recipe.Hash() == b.Recipe.Hash() && a.CreatedAt.Equal(b.CreatedAt) && a.ReserveMicroUSD == b.ReserveMicroUSD && a.Settled == b.Settled && a.CostMicroUSD == b.CostMicroUSD
}
