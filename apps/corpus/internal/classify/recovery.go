package classify

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go/v2"
)

func validID(id string) bool {
	b, err := hex.DecodeString(id)
	return err == nil && len(b) == 16 && strings.ToLower(id) == id
}

func (r *Runner) Rebuild(ctx context.Context) error {
	unlock, err := r.Repository.Lock(ctx)
	if err != nil {
		return err
	}
	defer unlock()
	rows, err := r.Repository.List(ctx)
	if err != nil {
		return err
	}
	existing := map[string]bool{}
	for _, s := range rows {
		existing[s.ID] = true
	}
	dir := filepath.Join(r.IntentDir, "submissions")
	files, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read submission intent log: %w", err)
	}
	for _, f := range files {
		if !strings.HasSuffix(f.Name(), ".json") {
			continue
		}
		id := strings.TrimSuffix(f.Name(), ".json")
		if !validID(id) || !f.Type().IsRegular() {
			return fmt.Errorf("invalid submission intent path")
		}
		b, readErr := os.ReadFile(filepath.Join(dir, f.Name()))
		if readErr != nil {
			return fmt.Errorf("read submission intent: %w", readErr)
		}
		var s Submission
		if err = json.Unmarshal(b, &s); err != nil {
			return fmt.Errorf("decode submission intent: %w", err)
		}
		if s.ID != id || s.ItemsHash != itemsHash(s.Items) || s.Recipe.Validate() != nil || s.ReserveMicroUSD < 0 || len(s.Items) == 0 || len(s.Items) > 100000 {
			return fmt.Errorf("invalid submission intent identity or ordered item hash")
		}
		var reserve int64
		for _, item := range s.Items {
			cost, reserveErr := s.Recipe.Reserve(item.InputTokens)
			if reserveErr != nil {
				return fmt.Errorf("validate recovered reservation: %w", reserveErr)
			}
			reserve += cost
		}
		if reserve != s.ReserveMicroUSD {
			return fmt.Errorf("recovered submission reservation differs from counted input and output bounds")
		}
		if existing[id] {
			continue
		}
		s.State = "uncertain"
		s.Settled = false
		s.Results = map[string]Outcome{}
		s.CostMicroUSD = 0
		batch, readErr := os.ReadFile(filepath.Join(dir, id+".batch"))
		if readErr == nil {
			var record batchRecord
			if err = json.Unmarshal(batch, &record); err != nil {
				return fmt.Errorf("decode provider batch identity: %w", err)
			}
			if record.ID == "" || record.SubmittedAt.Before(s.CreatedAt.Add(-time.Minute)) || !record.DeadlineAt.Equal(record.SubmittedAt.Add(26*time.Hour)) {
				return fmt.Errorf("invalid provider batch identity")
			}
			s.BatchID = record.ID
			s.SubmittedAt = &record.SubmittedAt
			s.DeadlineAt = &record.DeadlineAt
			s.State = "submitted"
		} else if !errors.Is(readErr, os.ErrNotExist) {
			return fmt.Errorf("read provider batch identity: %w", readErr)
		}
		if err = r.Repository.Save(ctx, s); err != nil {
			return err
		}
	}
	return nil
}

type batchLister interface {
	List(context.Context, time.Time, time.Time) ([]anthropic.MessageBatch, error)
}

func (r *Runner) Reconcile(ctx context.Context, id string, abandon bool) error {
	if !validID(id) {
		return fmt.Errorf("invalid submission id")
	}
	if r.Now == nil {
		r.Now = time.Now
	}
	unlock, err := r.Repository.Lock(ctx)
	if err != nil {
		return err
	}
	defer unlock()
	rows, err := r.Repository.List(ctx)
	if err != nil {
		return err
	}
	var target *Submission
	for i := range rows {
		if rows[i].ID == id {
			target = &rows[i]
		}
	}
	if target == nil || target.State != "uncertain" {
		return fmt.Errorf("only uncertain submissions can be reconciled")
	}
	lister, ok := r.Client.(batchLister)
	if !ok {
		return fmt.Errorf("provider batch listing is unavailable")
	}
	batches, err := lister.List(ctx, target.CreatedAt.Add(-time.Minute), target.CreatedAt.Add(10*time.Minute))
	if err != nil {
		return providerError(err)
	}
	found := ""
	var matchedAt time.Time
	allEnded := true
	for _, b := range batches {
		if b.ProcessingStatus != "ended" {
			allEnded = false
			continue
		}
		results, resultErr := r.Client.Results(ctx, b.ID)
		if resultErr != nil {
			return providerError(resultErr)
		}
		for _, result := range results {
			if strings.HasPrefix(result.CustomID, id+"-") {
				if found != "" && found != b.ID {
					return fmt.Errorf("multiple batches match submission; operator review required")
				}
				found = b.ID
				matchedAt = b.CreatedAt
			}
		}
	}
	if found != "" {
		if !matchedAt.IsZero() {
			target.SubmittedAt = &matchedAt
		}
		return r.attach(ctx, target, found)
	}
	if !abandon {
		return fmt.Errorf("no ended provider batch matches submission")
	}
	if !allEnded || r.Now().Before(target.CreatedAt.Add(24*time.Hour)) {
		return fmt.Errorf("cannot abandon until 24 hours pass and every candidate batch has ended")
	}
	target.State = "abandoned"
	return r.Repository.Save(ctx, *target)
}

type batchCanceller interface {
	Cancel(context.Context, string) error
}

func (r *Runner) Cancel(ctx context.Context, id, out string) error {
	if !validID(id) || out == "" {
		return fmt.Errorf("cancel requires a submission id and output directory")
	}
	if r.Now == nil {
		r.Now = time.Now
	}
	if r.Wait == nil {
		r.Wait = wait
	}
	unlock, err := r.Repository.Lock(ctx)
	if err != nil {
		return err
	}
	defer unlock()
	rows, err := r.Repository.List(ctx)
	if err != nil {
		return err
	}
	for _, s := range rows {
		if s.ID != id {
			continue
		}
		if s.State == "cancelled" {
			return writeFamilies(out, s)
		}
		if s.State != "submitted" && s.State != "cancelling" {
			return fmt.Errorf("only in-flight submissions can be cancelled")
		}
		canceller, ok := r.Client.(batchCanceller)
		if !ok {
			return fmt.Errorf("provider batch cancellation unavailable")
		}
		if err = canceller.Cancel(ctx, s.BatchID); err != nil {
			return providerError(err)
		}
		s.State = "cancelling"
		if err = r.Repository.Save(ctx, s); err != nil {
			return err
		}
		return r.poll(ctx, &s, out)
	}
	return fmt.Errorf("submission not found")
}
