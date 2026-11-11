package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"gopkg.in/yaml.v3"
)

type exportReport struct {
	ReportID       int64             `json:"report_id"`
	CreatedAt      time.Time         `json:"created_at"`
	DocumentKey    string            `json:"document_key"`
	Anchor         string            `json:"anchor"`
	DocumentNumber string            `json:"document_number"`
	Release        string            `json:"release"`
	Kind           string            `json:"kind"`
	Target         string            `json:"target"`
	Suggestion     map[string]string `json:"suggestion"`
}
type exportGroup struct {
	DocumentKey string         `json:"document_key"`
	Anchor      string         `json:"anchor"`
	Kind        string         `json:"kind"`
	Target      string         `json:"target"`
	Reports     []exportReport `json:"reports"`
}
type draftCorrection struct {
	Key      string            `yaml:"key"`
	Number   string            `yaml:"number"`
	Set      map[string]string `yaml:"set"`
	Reason   string            `yaml:"reason"`
	Evidence map[string]string `yaml:"evidence"`
	Reviewed string            `yaml:"reviewed"`
}

func writeReportExport(dir, id string, rows []exportReport) error {
	groups := map[string]*exportGroup{}
	var keys []string
	ids := make([]int64, 0, len(rows))
	created := rows[0].CreatedAt
	for _, r := range rows {
		key := r.DocumentKey + "\x1f" + r.Anchor + "\x1f" + r.Kind + "\x1f" + r.Target
		g := groups[key]
		if g == nil {
			g = &exportGroup{DocumentKey: r.DocumentKey, Anchor: r.Anchor, Kind: r.Kind, Target: r.Target}
			groups[key] = g
			keys = append(keys, key)
		}
		g.Reports = append(g.Reports, r)
		ids = append(ids, r.ReportID)
	}
	sort.Strings(keys)
	drafts := []draftCorrection{}
	var validity, text, citation bytes.Buffer
	for _, key := range keys {
		g := groups[key]
		switch g.Kind {
		case "wrong_validity":
			same := len(g.Reports[0].Suggestion) > 0
			first, _ := json.Marshal(g.Reports[0].Suggestion)
			for _, r := range g.Reports[1:] {
				v, _ := json.Marshal(r.Suggestion)
				if !bytes.Equal(v, first) {
					same = false
				}
			}
			if g.Anchor == "" && same {
				drafts = append(drafts, draftCorrection{Key: g.DocumentKey, Number: g.Reports[0].DocumentNumber, Set: g.Reports[0].Suggestion, Reason: "TODO", Evidence: map[string]string{"quote": "TODO"}, Reviewed: ""})
			} else if err := json.NewEncoder(&validity).Encode(g); err != nil {
				return fmt.Errorf("encode validity: %w", err)
			}
		case "wrong_text":
			if err := json.NewEncoder(&text).Encode(g); err != nil {
				return fmt.Errorf("encode text: %w", err)
			}
		case "broken_citation", "missing_document":
			if err := json.NewEncoder(&citation).Encode(g); err != nil {
				return fmt.Errorf("encode citation: %w", err)
			}
		}
	}
	draft, err := yaml.Marshal(drafts)
	if err != nil {
		return fmt.Errorf("encode drafts: %w", err)
	}
	manifest, err := json.MarshalIndent(struct {
		BatchID   string    `json:"batch_id"`
		CreatedAt time.Time `json:"created_at"`
		ReportIDs []int64   `json:"report_ids"`
	}{id, created, ids}, "", "  ")
	if err != nil {
		return fmt.Errorf("encode manifest: %w", err)
	}
	files := map[string][]byte{"corrections-draft.yaml": draft, "validity-review.jsonl": validity.Bytes(), "text-issues.jsonl": text.Bytes(), "citation-review.jsonl": citation.Bytes(), "manifest.json": append(manifest, '\n')}
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0600); err != nil {
			return fmt.Errorf("write export: %w", err)
		}
	}
	return nil
}
