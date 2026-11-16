package classify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

func writeOnce(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create submission directory: %w", err)
	}
	f, err := os.CreateTemp(dir, ".submission-")
	if err != nil {
		return fmt.Errorf("create submission file: %w", err)
	}
	defer func() { _ = os.Remove(f.Name()) }()
	if _, err = f.Write(data); err != nil {
		_ = f.Close()
		return fmt.Errorf("write submission file: %w", err)
	}
	if err = f.Sync(); err != nil {
		_ = f.Close()
		return fmt.Errorf("sync submission file: %w", err)
	}
	if err = f.Close(); err != nil {
		return fmt.Errorf("close submission file: %w", err)
	}
	if err = os.Link(f.Name(), path); err != nil {
		if !errors.Is(err, os.ErrExist) {
			return fmt.Errorf("publish submission file: %w", err)
		}
		b, readErr := os.ReadFile(path)
		if readErr != nil {
			return fmt.Errorf("read existing submission file: %w", readErr)
		}
		if !bytes.Equal(b, data) {
			return fmt.Errorf("existing submission file differs")
		}
	}
	d, err := os.Open(dir)
	if err != nil {
		return fmt.Errorf("open submission directory: %w", err)
	}
	err = d.Sync()
	closeErr := d.Close()
	if err != nil {
		return fmt.Errorf("sync submission directory: %w", err)
	}
	if closeErr != nil {
		return fmt.Errorf("close submission directory: %w", closeErr)
	}
	return nil
}

func (r *Runner) writeIntent(s Submission) error {
	if r.IntentDir == "" {
		return fmt.Errorf("submission intent directory is required")
	}
	b, err := json.Marshal(s)
	if err != nil {
		return fmt.Errorf("encode submission intent: %w", err)
	}
	return writeOnce(filepath.Join(r.IntentDir, "submissions", s.ID+".json"), b)
}

type batchRecord struct {
	ID          string    `json:"provider_job_id"`
	SubmittedAt time.Time `json:"submitted_at"`
	DeadlineAt  time.Time `json:"deadline_at"`
}

func (r *Runner) writeBatch(s Submission) error {
	if s.SubmittedAt == nil || s.DeadlineAt == nil || s.BatchID == "" {
		return fmt.Errorf("missing provider batch identity")
	}
	b, err := json.Marshal(batchRecord{ID: s.BatchID, SubmittedAt: *s.SubmittedAt, DeadlineAt: *s.DeadlineAt})
	if err != nil {
		return fmt.Errorf("encode batch identity: %w", err)
	}
	return writeOnce(filepath.Join(r.IntentDir, "submissions", s.ID+".batch"), b)
}

func (r *Runner) attach(ctx context.Context, s *Submission, id string) error {
	now := s.CreatedAt
	if s.SubmittedAt != nil {
		now = s.SubmittedAt.UTC().Truncate(time.Microsecond)
	}
	deadline := now.Add(26 * time.Hour)
	s.BatchID = id
	s.State = "submitted"
	s.SubmittedAt = &now
	s.DeadlineAt = &deadline
	if err := r.writeBatch(*s); err != nil {
		return err
	}
	return r.Repository.Save(ctx, *s)
}

func (r *Runner) restoreReceipt(s *Submission) (bool, error) {
	b, err := os.ReadFile(filepath.Join(r.IntentDir, "submissions", s.ID+".batch"))
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read provider batch identity: %w", err)
	}
	var record batchRecord
	if err = json.Unmarshal(b, &record); err != nil {
		return false, fmt.Errorf("decode provider batch identity: %w", err)
	}
	if record.ID == "" || record.SubmittedAt.Before(s.CreatedAt.Add(-time.Minute)) || !record.DeadlineAt.Equal(record.SubmittedAt.Add(26*time.Hour)) {
		return false, fmt.Errorf("invalid provider batch identity")
	}
	s.BatchID = record.ID
	s.SubmittedAt = &record.SubmittedAt
	s.DeadlineAt = &record.DeadlineAt
	s.State = "submitted"
	return true, nil
}

func writeFamilies(dir string, s Submission) error {
	artifact := struct {
		Version      int                `json:"version"`
		RecipeHash   string             `json:"recipe_hash"`
		TaxonomyHash string             `json:"taxonomy_hash"`
		Recipe       Recipe             `json:"recipe"`
		Documents    map[string]Outcome `json:"documents"`
	}{1, s.Recipe.Hash(), s.Recipe.TaxonomyHash, s.Recipe, s.Results}
	b, err := json.MarshalIndent(artifact, "", "  ")
	if err != nil {
		return fmt.Errorf("encode family artifact: %w", err)
	}
	b = append(b, '\n')
	if err = writeOnce(filepath.Join(dir, "families.json"), b); err != nil {
		return err
	}
	return writeOnce(filepath.Join(dir, "MANIFEST.sha256"), []byte(digest(b)+"  families.json\n"))
}
