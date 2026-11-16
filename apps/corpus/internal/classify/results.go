package classify

import (
	"context"
	"errors"
	"fmt"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/batchapi"
	"github.com/anthropics/anthropic-sdk-go/v2"
)

func providerError(err error) error {
	var api *anthropic.Error
	if errors.As(err, &api) {
		return fmt.Errorf("classification provider request failed with HTTP status %d", api.StatusCode)
	}
	if errors.Is(err, context.Canceled) {
		return fmt.Errorf("classification provider request interrupted: %w", context.Canceled)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("classification provider request timed out: %w", context.DeadlineExceeded)
	}
	return fmt.Errorf("classification provider request failed")
}

func (r *Runner) poll(ctx context.Context, s *Submission, out string) error {
	if s.DeadlineAt == nil {
		return fmt.Errorf("submission has no polling deadline")
	}
	if err := r.writeBatch(*s); err != nil {
		return err
	}
	for {
		current, err := r.transition(ctx, s.ID, func(latest *Submission) (bool, error) {
			if settled(*latest) {
				return false, nil
			}
			if stateErr := pollingState(latest); stateErr != nil {
				return false, stateErr
			}
			if !r.Now().Before(*latest.DeadlineAt) {
				latest.State = "failed"
				return true, nil
			}
			return false, nil
		})
		if err != nil {
			return err
		}
		if settled(current) {
			return writeFamilies(out, current)
		}
		if err = pollingState(&current); err != nil {
			return err
		}
		b, err := r.Client.Get(ctx, current.BatchID)
		if err != nil {
			return providerError(err)
		}
		if b.ProcessingStatus == "ended" {
			results, resultErr := r.Client.Results(ctx, current.BatchID)
			if resultErr != nil {
				return providerError(resultErr)
			}
			finished, finishErr := r.transition(ctx, s.ID, func(latest *Submission) (bool, error) {
				if settled(*latest) {
					return false, nil
				}
				if stateErr := pollingState(latest); stateErr != nil {
					return false, stateErr
				}
				if applyErr := apply(latest, results); applyErr != nil {
					return false, applyErr
				}
				return true, nil
			})
			if finishErr != nil {
				return finishErr
			}
			return writeFamilies(out, finished)
		}
		delay := min(5*time.Minute, current.DeadlineAt.Sub(r.Now()))
		if delay <= 0 {
			continue
		}
		if err = r.Wait(ctx, delay); err != nil {
			return fmt.Errorf("wait for classification batch: %w", err)
		}
	}
}

func apply(s *Submission, results []batchapi.Result) error {
	byID := map[string]batchapi.Result{}
	for _, result := range results {
		if _, exists := byID[result.CustomID]; exists {
			return fmt.Errorf("duplicate classification custom_id")
		}
		byID[result.CustomID] = result
	}
	if len(byID) != len(s.Items) {
		return fmt.Errorf("classification batch results have incomplete or foreign identities; reservation retained")
	}
	outcomes := map[string]Outcome{}
	var input, output int64
	for i, item := range s.Items {
		v, ok := byID[customID(s.ID, i)]
		if !ok {
			return fmt.Errorf("classification batch result omits an item; reservation retained")
		}
		out := Outcome{Outcome: "invalid_response", Codes: []string{}, Dropped: []string{}}
		switch v.Type {
		case "succeeded":
			m := v.Message
			u := m.Usage
			if !u.JSON.InputTokens.Valid() || !u.JSON.OutputTokens.Valid() || u.InputTokens < 0 || u.OutputTokens < 0 || u.InputTokens > 100000 || u.OutputTokens > s.Recipe.outputTokens() || u.CacheReadInputTokens != 0 || u.CacheCreationInputTokens != 0 {
				return fmt.Errorf("classification usage is incomplete or outside verified pricing; reservation retained")
			}
			input += u.InputTokens
			output += u.OutputTokens
			var text string
			for _, content := range m.Content {
				if content.Type == "text" {
					text += content.Text
				}
			}
			if s.Recipe.Article != nil {
				if item.Article == nil {
					return fmt.Errorf("article submission is missing request evidence; reservation retained")
				}
				out.Articles, out.DroppedAnchors = ValidateArticles(string(m.StopReason), text, *item.Article, item.Article.Nodes)
			} else {
				out = Validate(string(m.StopReason), text, s.Recipe.F, s.Families)
			}
		case "errored", "expired", "canceled":
		default:
			return fmt.Errorf("unknown classification result type; reservation retained")
		}
		outcomes[item.Key] = out
	}
	s.Results = outcomes
	s.CostMicroUSD = s.Recipe.cost(input, output)
	s.Settled = true
	if s.State == "cancelling" {
		s.State = "cancelled"
	} else {
		s.State = "completed"
	}
	return nil
}
