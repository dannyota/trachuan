package classify

import (
	"context"
	"fmt"
)

func (r *Runner) transition(ctx context.Context, id string, update func(*Submission) (bool, error)) (Submission, error) {
	unlock, err := r.Repository.Lock(ctx)
	if err != nil {
		return Submission{}, err
	}
	defer unlock()
	rows, err := r.Repository.List(ctx)
	if err != nil {
		return Submission{}, err
	}
	for _, s := range rows {
		if s.ID != id {
			continue
		}
		changed, updateErr := update(&s)
		if updateErr != nil {
			return Submission{}, updateErr
		}
		if changed {
			if err = r.Repository.Save(ctx, s); err != nil {
				return Submission{}, err
			}
		}
		return s, nil
	}
	return Submission{}, fmt.Errorf("submission not found")
}

func settled(s Submission) bool {
	return s.Settled && (s.State == "completed" || s.State == "cancelled")
}

func pollingState(s *Submission) error {
	if s.State == "failed" {
		return fmt.Errorf("classification batch exceeded its 26-hour deadline; reservation retained for operator review")
	}
	if s.State != "submitted" && s.State != "cancelling" {
		return fmt.Errorf("classification submission is %s; operator review required", s.State)
	}
	if s.DeadlineAt == nil {
		return fmt.Errorf("submission has no polling deadline")
	}
	return nil
}
