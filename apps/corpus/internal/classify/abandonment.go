package classify

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type abandonmentRecord struct {
	ID          string    `json:"id"`
	ItemsHash   string    `json:"items_hash"`
	AbandonedAt time.Time `json:"abandoned_at"`
}

func (r *Runner) writeAbandonment(s *Submission) error {
	known, err := r.restoreAbandonment(s)
	if err != nil || known {
		return err
	}
	b, err := json.Marshal(abandonmentRecord{ID: s.ID, ItemsHash: s.ItemsHash, AbandonedAt: r.Now().UTC().Truncate(time.Microsecond)})
	if err != nil {
		return fmt.Errorf("encode submission abandonment: %w", err)
	}
	return writeOnce(filepath.Join(r.IntentDir, "submissions", s.ID+".abandoned"), b)
}

func (r *Runner) restoreAbandonment(s *Submission) (bool, error) {
	b, err := os.ReadFile(filepath.Join(r.IntentDir, "submissions", s.ID+".abandoned"))
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read submission abandonment: %w", err)
	}
	var record abandonmentRecord
	if err = json.Unmarshal(b, &record); err != nil {
		return false, fmt.Errorf("decode submission abandonment: %w", err)
	}
	if record.ID != s.ID || record.ItemsHash != s.ItemsHash || record.AbandonedAt.Before(s.CreatedAt.Add(24*time.Hour)) {
		return false, fmt.Errorf("invalid submission abandonment")
	}
	s.State = "abandoned"
	return true, nil
}

func (r *Runner) requireReconstruction(rows []Submission) error {
	known := make(map[string]bool, len(rows))
	for _, s := range rows {
		known[s.ID] = true
	}
	files, err := os.ReadDir(filepath.Join(r.IntentDir, "submissions"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect submission intents before admission: %w", err)
	}
	for _, file := range files {
		if filepath.Ext(file.Name()) != ".json" {
			continue
		}
		id := file.Name()[:len(file.Name())-len(".json")]
		if !validID(id) || !file.Type().IsRegular() {
			return fmt.Errorf("invalid submission intent path")
		}
		if !known[id] {
			return fmt.Errorf("submission ledger omits retained intents; run corpus submissions --rebuild before admission")
		}
	}
	return nil
}
