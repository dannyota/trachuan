package build

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"danny.vn/trachuan/apps/corpus/internal/knowledge"
	"danny.vn/trachuan/apps/corpus/internal/stage"
)

const (
	knowledgeFileName   = "knowledge.yaml"
	knowledgeReportName = "knowledge-report.json"
)

type KnowledgeInput struct {
	file knowledge.File
	data []byte
	hash string
}

func ReadKnowledge(country, path string) (KnowledgeInput, error) {
	file, data, err := knowledge.ParseFile(country, path)
	if err != nil {
		return KnowledgeInput{}, err
	}
	if file.Len() == 0 {
		return KnowledgeInput{}, nil
	}
	return KnowledgeInput{file: file, data: data, hash: hashBytes(data)}, nil
}

func (k KnowledgeInput) Empty() bool { return k.file.Len() == 0 }

func withKnowledge(input, hash string) string {
	if hash == "" {
		return input
	}
	return stage.RecipeHash("build-knowledge", knowledge.RulesVersion, input, hash)
}

type knowledgeReader struct {
	knowledge.SQLReader
	gaps []InventoryGap
}

func (r knowledgeReader) BaselineServed(_ context.Context, keys []string) ([]string, error) {
	wanted := make(map[string]bool, len(keys))
	for _, key := range keys {
		wanted[key] = true
	}
	served := []string{}
	for _, gap := range r.gaps {
		if gap.Stage == "build" && gap.Kind == "baseline_served" && wanted[gap.ItemKey] {
			served = append(served, gap.ItemKey)
		}
	}
	return served, nil
}

func (l *loader) loadKnowledge(ctx context.Context, input KnowledgeInput) error {
	report, err := knowledge.Check(ctx, knowledgeReader{SQLReader: knowledge.SQLReader{Q: l.tx}, gaps: l.gaps}, input.file)
	if err != nil {
		return fmt.Errorf("check knowledge: %w", err)
	}
	if err = report.Err(); err != nil {
		return err
	}
	for _, record := range report.Records {
		if record.Outcome == knowledge.OutcomeStale {
			l.addGap("build", record.TargetKey, "knowledge_stale", record.ID+":"+record.Reason)
		}
	}
	for _, r := range report.Rows {
		_, err = l.tx.Exec(ctx, `INSERT INTO relation(id,source_document_id,target_document_id,source_key,target_key,source_number,target_number,kind,raw_code,raw_label,source_provision_id,target_provision_id,source_node_id,target_node_id,evidence,method,artifact_sha256,ordinal) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)`, l.counts.Relations+1, r.SourceDocumentID, r.TargetDocumentID, r.SourceKey, r.TargetKey, r.SourceNumber, r.TargetNumber, r.Kind, knowledge.MethodReviewed, r.Label, r.SourceProvisionID, r.TargetProvisionID, r.SourceNodeID, r.TargetNodeID, r.Evidence, knowledge.MethodReviewed, input.hash, r.Ordinal)
		if err != nil {
			return fmt.Errorf("insert knowledge relation: %w", err)
		}
		l.counts.Relations++
	}
	l.counts.knowledge = report
	return nil
}

func ruleRelations(relations []validityRelation) []validityRelation {
	rules := make([]validityRelation, 0, len(relations))
	for _, r := range relations {
		if r.method != knowledge.MethodReviewed {
			rules = append(rules, r)
		}
	}
	return rules
}

func writeKnowledgeArtifact(dir string, input KnowledgeInput, report knowledge.Report) error {
	if input.Empty() {
		return nil
	}
	if err := os.WriteFile(filepath.Join(dir, knowledgeFileName), input.data, 0600); err != nil {
		return fmt.Errorf("write knowledge: %w", err)
	}
	records := report.Records
	if records == nil {
		records = []knowledge.Record{}
	}
	data, err := json.Marshal(records)
	if err != nil {
		return fmt.Errorf("encode knowledge report: %w", err)
	}
	if err = os.WriteFile(filepath.Join(dir, knowledgeReportName), data, 0600); err != nil {
		return fmt.Errorf("write knowledge report: %w", err)
	}
	return nil
}

func knowledgeArtifactHash(dir string) (string, error) {
	data, err := os.ReadFile(filepath.Join(dir, knowledgeFileName))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Errorf("read knowledge: %w", err)
	}
	present := err == nil
	_, err = os.Stat(filepath.Join(dir, knowledgeReportName))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Errorf("stat knowledge report: %w", err)
	}
	if present != (err == nil) {
		return "", fmt.Errorf("knowledge file and report must appear together")
	}
	if !present {
		return "", nil
	}
	return hashBytes(data), nil
}
