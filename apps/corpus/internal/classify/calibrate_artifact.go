package classify

import (
	"encoding/json"
	"fmt"
	"path/filepath"
)

func WriteCalibration(dir string, report CalibrationReport) error {
	b, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("encode calibration: %w", err)
	}
	b = append(b, '\n')
	if err = writeOnce(filepath.Join(dir, "calibration.json"), b); err != nil {
		return err
	}
	return writeOnce(filepath.Join(dir, "MANIFEST.sha256"), []byte(digest(b)+"  calibration.json\n"))
}
