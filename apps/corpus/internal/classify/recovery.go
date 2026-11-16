package classify

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
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
	existing := map[string]Submission{}
	for _, s := range rows {
		existing[s.ID] = s
	}
	dir := filepath.Join(r.IntentDir, "submissions")
	files, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read submission intent log: %w", err)
	}
	var recovered []Submission
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
		abandoned, restoreErr := r.restoreAbandonment(&s)
		if restoreErr != nil {
			return restoreErr
		}
		if prior, ok := existing[id]; ok {
			if abandoned && nonterminal(prior.State) {
				prior.State = "abandoned"
				recovered = append(recovered, prior)
			}
			continue
		}
		recovered = append(recovered, s)
	}
	sort.SliceStable(recovered, func(i, j int) bool { return !nonterminal(recovered[i].State) && nonterminal(recovered[j].State) })
	return r.Repository.Restore(ctx, recovered)
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
	canceller, ok := r.Client.(batchCanceller)
	s, err := r.transition(ctx, id, func(latest *Submission) (bool, error) {
		if latest.State == "cancelled" {
			return false, nil
		}
		if stateErr := pollingState(latest); stateErr != nil {
			return false, stateErr
		}
		if !ok {
			return false, fmt.Errorf("provider batch cancellation unavailable")
		}
		latest.State = "cancelling"
		return true, nil
	})
	if err != nil {
		return err
	}
	if s.State == "cancelled" {
		return writeFamilies(out, s)
	}
	if err = canceller.Cancel(ctx, s.BatchID); err != nil {
		return providerError(err)
	}
	return r.poll(ctx, &s, out)
}
