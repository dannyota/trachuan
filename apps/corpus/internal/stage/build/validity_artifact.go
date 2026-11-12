package build

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"danny.vn/trachuan/apps/corpus/internal/source"
)

type reviewArtifactEntry struct {
	Kind              string               `json:"kind"`
	Key               string               `json:"key"`
	Number            string               `json:"number"`
	TargetKey         string               `json:"target_key"`
	TargetNumber      string               `json:"target_number"`
	Detail            string               `json:"detail"`
	StatusCode        string               `json:"status_code"`
	StatusName        string               `json:"status_name"`
	RawFrom           string               `json:"raw_eff_from"`
	RawTo             string               `json:"raw_eff_to"`
	TargetStatusCode  string               `json:"target_status_code"`
	TargetStatusName  string               `json:"target_status_name"`
	TargetRawFrom     string               `json:"target_raw_eff_from"`
	TargetRawTo       string               `json:"target_raw_eff_to"`
	RelationID        int64                `json:"relation_id,omitempty"`
	Sentences         []string             `json:"sentences"`
	FetchHash         string               `json:"fetch_sha256"`
	ExtractHash       string               `json:"extract_sha256"`
	TargetFetchHash   string               `json:"target_fetch_sha256"`
	TargetExtractHash string               `json:"target_extract_sha256"`
	Proposed          *canonicalCorrection `json:"proposed"`
}

func encodeValidityReview(reviews []validityReview, partial ...[]partialDocument) ([]byte, error) {
	entries := make([]reviewArtifactEntry, 0, len(reviews))
	for _, r := range reviews {
		entry := reviewArtifactEntry{Kind: r.kind, Key: r.key, Number: r.number, TargetKey: r.targetKey, TargetNumber: r.targetNumber, Detail: r.detail, StatusCode: r.statusCode, StatusName: r.statusName, RawFrom: r.rawFrom, RawTo: r.rawTo, TargetStatusCode: r.targetStatusCode, TargetStatusName: r.targetStatusName, TargetRawFrom: r.targetRawFrom, TargetRawTo: r.targetRawTo, RelationID: r.relationID, Sentences: append([]string{}, r.sentences...), FetchHash: r.fetchHash, ExtractHash: r.extractHash, TargetFetchHash: r.targetFetchHash, TargetExtractHash: r.targetExtractHash}
		if r.proposed != nil {
			raw, err := canonicalCorrections([]source.Correction{*r.proposed})
			if err != nil {
				return nil, fmt.Errorf("encode proposed correction: %w", err)
			}
			var values []canonicalCorrection
			if err = json.Unmarshal(raw, &values); err != nil {
				return nil, fmt.Errorf("decode proposed correction: %w", err)
			}
			entry.Proposed = &values[0]
		}
		entries = append(entries, entry)
	}
	documents := []partialDocument{}
	if len(partial) > 0 && partial[0] != nil {
		documents = partial[0]
	}
	data, err := json.Marshal(struct {
		PartialDocuments []partialDocument     `json:"partial_documents"`
		Version          int                   `json:"version"`
		Entries          []reviewArtifactEntry `json:"entries"`
	}{documents, 1, entries})
	if err != nil {
		return nil, fmt.Errorf("encode validity review: %w", err)
	}
	return data, nil
}

func verifyValidityReview(dir string) error {
	data, err := os.ReadFile(filepath.Join(dir, "validity-review.json"))
	if err != nil {
		return fmt.Errorf("read validity review: %w", err)
	}
	var report struct {
		Version int               `json:"version"`
		Entries []json.RawMessage `json:"entries"`
	}
	if err = json.Unmarshal(data, &report); err != nil {
		return fmt.Errorf("decode validity review: %w", err)
	}
	if report.Version != 1 || report.Entries == nil {
		return fmt.Errorf("invalid validity review")
	}
	return nil
}
