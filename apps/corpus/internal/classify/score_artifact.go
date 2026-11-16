package classify

import (
	"encoding/json"
	"fmt"
	"path/filepath"
)

func WriteScore(dir string, report ScoreReport) error {
	b, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("encode classification scores: %w", err)
	}
	b = append(b, '\n')
	if err = writeOnce(filepath.Join(dir, "thresholds.json"), b); err != nil {
		return err
	}
	return writeOnce(filepath.Join(dir, "MANIFEST.sha256"), []byte(digest(b)+"  thresholds.json\n"))
}
