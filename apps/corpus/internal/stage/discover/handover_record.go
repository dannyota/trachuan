package discover

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

// HandOverComparison holds one candidate-specific probe comparison.
type HandOverComparison struct {
	ID, State            string
	Number, Issuer, Date *string
}

// HandOverFile identifies a source response bound by a candidate record.
type HandOverFile struct{ Path, SHA256 string }

// HandOverRecord holds the identity evidence needed at fetch close.
type HandOverRecord struct {
	State, ProbeState, Number, Issuer, Date, PrimaryID, Path, SHA256 string
	DetailPath                                                       string
	Comparisons                                                      []HandOverComparison
	Files                                                            []HandOverFile
}

// ReadHandOverRecord reads a candidate record from an already verified artifact.
func ReadHandOverRecord(dir, docid string) (HandOverRecord, error) {
	id, err := strconv.Atoi(docid)
	if err != nil || id <= 0 || strconv.Itoa(id) != docid {
		return HandOverRecord{}, fmt.Errorf("invalid secondary id %q", docid)
	}
	path := "candidate-" + docid + ".json"
	data, err := os.ReadFile(filepath.Join(dir, path))
	if err != nil {
		return HandOverRecord{}, fmt.Errorf("read candidate: %w", err)
	}
	var evidence secondaryEvidence
	if err := json.Unmarshal(data, &evidence); err != nil {
		return HandOverRecord{}, fmt.Errorf("decode candidate: %w", err)
	}
	c := evidence.Candidate
	if c.ID != docid {
		return HandOverRecord{}, fmt.Errorf("candidate id mismatch")
	}
	result := HandOverRecord{DetailPath: c.DetailPath, State: c.State, ProbeState: c.ProbeState, Number: c.Number, Issuer: c.IssuerForm, Date: c.Date, PrimaryID: c.PrimaryID, Path: path, SHA256: fmt.Sprintf("%x", sha256.Sum256(data))}
	if c.DetailPath != "" {
		result.Files = append(result.Files, HandOverFile{Path: c.DetailPath, SHA256: c.DetailHash})
	}
	for _, pass := range c.ProbePasses {
		for _, page := range pass.Evidence.PagesFetched {
			result.Files = append(result.Files, HandOverFile{Path: page.Path, SHA256: page.SHA256})
		}
	}
	for _, x := range c.Comparisons {
		result.Comparisons = append(result.Comparisons, HandOverComparison{ID: x.ID, State: x.State, Number: x.Item.RawNumber, Issuer: x.Item.Issuer, Date: x.Item.IssueDate})
	}
	return result, nil
}
