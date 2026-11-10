package build

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/source"
)

func TestValidityReview_EncodesEvidence(t *testing.T) {
	date := "2024-07-01"
	entry := validityReview{kind: "date_conflict", key: "vn:vbpl:1", number: "1/2024/NĐ-CP", statusCode: "CCHL", rawFrom: "2027-01-01", sentences: []string{"text"}, extractHash: "abc", proposed: &source.Correction{Key: "vn:vbpl:1", Number: "1/2024/NĐ-CP", Set: source.CorrectionSet{EffFrom: &date}}}
	data, err := encodeValidityReview([]validityReview{entry})
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		Entries []struct {
			Kind     string `json:"kind"`
			Key      string `json:"key"`
			Proposed *struct {
				Key string `json:"key"`
			} `json:"proposed"`
		} `json:"entries"`
	}
	if err = json.Unmarshal(data, &decoded); err != nil || len(decoded.Entries) != 1 || decoded.Entries[0].Kind != "date_conflict" || decoded.Entries[0].Key != "vn:vbpl:1" || decoded.Entries[0].Proposed == nil || decoded.Entries[0].Proposed.Key != "vn:vbpl:1" {
		t.Fatalf("review %s: %v", data, err)
	}
}

func TestValidityReview_RejectsMissingOrInvalidArtifact(t *testing.T) {
	dir := t.TempDir()
	if err := verifyValidityReview(dir); err == nil {
		t.Fatal("accepted missing review")
	}
	if err := os.WriteFile(filepath.Join(dir, "validity-review.json"), []byte(`{"version":1,"entries":null}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := verifyValidityReview(dir); err == nil {
		t.Fatal("accepted null entries")
	}
	if err := os.WriteFile(filepath.Join(dir, "validity-review.json"), []byte(`{"version":1,"entries":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := verifyValidityReview(dir); err != nil {
		t.Fatal(err)
	}
}
