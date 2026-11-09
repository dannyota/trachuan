package build

import (
	"crypto/sha256"
	"fmt"
	"testing"
)

func TestCalibration_RejectsThresholdMismatch(t *testing.T) {
	calibration := []byte(`{"threshold":0.2}`)
	recipe := []byte(fmt.Sprintf(`{"abstention":{"threshold":0.7,"calibration_sha256":"%x"}}`, sha256.Sum256(calibration)))
	if err := verifyCalibration(recipe, calibration); err == nil {
		t.Fatal("accepted threshold mismatch")
	}
}
