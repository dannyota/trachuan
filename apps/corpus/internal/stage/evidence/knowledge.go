package evidence

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/knowledge"
)

const (
	knowledgeFile = "knowledge.yaml"
	manifestFile  = "MANIFEST.sha256"
)

func readKnowledge(buildDir, manifestSHA256 string) ([]byte, error) {
	manifest, err := os.ReadFile(filepath.Join(buildDir, manifestFile))
	if err != nil || bareSHA256(manifest) != manifestSHA256 {
		return nil, refuse("build artifact failed verification")
	}
	path := filepath.Join(buildDir, knowledgeFile)
	if !manifestLists(string(manifest), knowledgeFile) {
		_, err = os.Lstat(path)
		if err == nil {
			return nil, refuse("build artifact holds a knowledge file its manifest does not list")
		}
		if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("inspect knowledge: %w", err)
		}
		return nil, nil
	}
	sum, err := artifact.VerifyFiles(buildDir, knowledgeFile)
	if err != nil || sum != manifestSHA256 {
		return nil, refuse("knowledge file failed verification")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read knowledge: %w", err)
	}
	return data, nil
}

func manifestLists(manifest, name string) bool {
	for line := range strings.SplitSeq(manifest, "\n") {
		if len(line) > 66 && line[64:66] == "  " && line[66:] == name {
			return true
		}
	}
	return false
}

func reviewDecisions(country string, data []byte) ([]ReviewDecision, error) {
	if data == nil {
		return nil, nil
	}
	file, err := knowledge.Parse(country, data)
	if err != nil {
		return nil, &IncompleteError{Reason: "knowledge: " + err.Error()}
	}
	decisions := make([]ReviewDecision, 0, len(file.Decisions))
	for _, d := range file.Decisions {
		decisions = append(decisions, ReviewDecision{ID: d.ID, Key: d.Finding, Fingerprint: d.Fingerprint, Semantics: d.Semantics, FirstRel: d.FirstRel})
	}
	return decisions, nil
}
