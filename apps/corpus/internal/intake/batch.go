package intake

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

const maxBatchFileBytes = 32 << 20

var batchIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

type manifestFile struct {
	BatchID   string  `json:"batch_id"`
	CreatedAt string  `json:"created_at"`
	ReportIDs []int64 `json:"report_ids"`
}

type batchReport struct {
	ReportID       int64             `json:"report_id"`
	CreatedAt      string            `json:"created_at"`
	DocumentKey    string            `json:"document_key"`
	Anchor         string            `json:"anchor"`
	DocumentNumber string            `json:"document_number"`
	Release        string            `json:"release"`
	Kind           string            `json:"kind"`
	Target         string            `json:"target"`
	Suggestion     map[string]string `json:"suggestion"`
}

type batchGroup struct {
	DocumentKey string        `json:"document_key"`
	Anchor      string        `json:"anchor"`
	Kind        string        `json:"kind"`
	Target      string        `json:"target"`
	Reports     []batchReport `json:"reports"`
}

type batchDraft struct {
	Key      string            `yaml:"key"`
	Number   string            `yaml:"number"`
	Set      map[string]string `yaml:"set"`
	Reason   string            `yaml:"reason"`
	Evidence map[string]string `yaml:"evidence"`
	Reviewed string            `yaml:"reviewed"`
}

var groupFiles = map[string][]string{
	"validity-review.jsonl": {"wrong_validity"},
	"text-issues.jsonl":     {"wrong_text"},
	"citation-review.jsonl": {"broken_citation", "missing_document"},
}

func readBatchFile(dir, name string) ([]byte, error) {
	path := filepath.Join(dir, name)
	info, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("batch file %s: %w", name, err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("batch file %s is not a regular file", name)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open batch file %s: %w", name, err)
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, maxBatchFileBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read batch file %s: %w", name, err)
	}
	if len(data) > maxBatchFileBytes {
		return nil, fmt.Errorf("batch file %s exceeds %d MiB", name, maxBatchFileBytes>>20)
	}
	return data, nil
}

func strictJSON(data []byte, into any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(into); err != nil {
		return err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return errors.New("data after the first JSON value")
	}
	return nil
}

func GroupID(key, anchor, kind, target string) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{key, anchor, kind, target}, "\x1f")))
	return hex.EncodeToString(sum[:])
}

func ReadBatch(country, dir string) ([]Record, error) {
	info, err := os.Lstat(dir)
	if err != nil {
		return nil, fmt.Errorf("inspect batch: %w", err)
	}
	if !info.IsDir() {
		return nil, errors.New("batch is not a directory")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("list batch: %w", err)
	}
	want := []string{"citation-review.jsonl", "corrections-draft.yaml", "manifest.json", "text-issues.jsonl", "validity-review.jsonl"}
	got := make([]string, 0, len(entries))
	for _, e := range entries {
		got = append(got, e.Name())
	}
	if !slices.Equal(got, want) {
		return nil, fmt.Errorf("batch must hold exactly %s", strings.Join(want, ", "))
	}
	data, err := readBatchFile(dir, "manifest.json")
	if err != nil {
		return nil, err
	}
	var manifest manifestFile
	if err = strictJSON(data, &manifest); err != nil {
		return nil, fmt.Errorf("decode manifest: %w", err)
	}
	if manifest.BatchID != filepath.Base(dir) || !batchIDPattern.MatchString(manifest.BatchID) {
		return nil, errors.New("manifest batch id does not match the directory")
	}
	listed := map[int64]bool{}
	for _, id := range manifest.ReportIDs {
		if id < 1 || listed[id] {
			return nil, errors.New("manifest report ids must be positive and unique")
		}
		listed[id] = true
	}
	if len(listed) == 0 {
		return nil, errors.New("manifest lists no reports")
	}
	var records []Record
	groups := map[string]bool{}
	for _, name := range []string{"citation-review.jsonl", "text-issues.jsonl", "validity-review.jsonl"} {
		data, err = readBatchFile(dir, name)
		if err != nil {
			return nil, err
		}
		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.DisallowUnknownFields()
		for {
			var g batchGroup
			if err = decoder.Decode(&g); errors.Is(err, io.EOF) {
				break
			} else if err != nil {
				return nil, fmt.Errorf("decode %s: %w", name, err)
			}
			if err = checkGroup(name, g, listed); err != nil {
				return nil, err
			}
			id := GroupID(g.DocumentKey, g.Anchor, g.Kind, g.Target)
			if groups[id] {
				return nil, fmt.Errorf("%s repeats a group", name)
			}
			groups[id] = true
			records = append(records, groupRecord(id, g))
		}
	}
	data, err = readBatchFile(dir, "corrections-draft.yaml")
	if err != nil {
		return nil, err
	}
	var drafts []batchDraft
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err = decoder.Decode(&drafts); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("decode corrections draft: %w", err)
	}
	var further yaml.Node
	if err = decoder.Decode(&further); !errors.Is(err, io.EOF) {
		return nil, errors.New("corrections draft holds more than one YAML document")
	}
	for _, d := range drafts {
		id := GroupID(d.Key, "", "wrong_validity", "")
		if groups[id] {
			return nil, errors.New("corrections draft repeats a group")
		}
		groups[id] = true
		records = append(records, Record{Origin: "issue_report", OriginID: id, Kind: "wrong_validity", Target: Target{Key: d.Key}, Claim: Claim{Code: "status_conflict"}})
	}
	if left := len(listed); (len(drafts) == 0 && left != 0) || left < len(drafts) {
		return nil, fmt.Errorf("manifest has %d reports outside the group files for %d drafts", left, len(drafts))
	}
	slices.SortFunc(records, compare)
	for _, r := range records {
		if problems := r.validate(country); len(problems) > 0 {
			return nil, fmt.Errorf("group %s: %s", r.OriginID[:12], strings.Join(problems, "; "))
		}
	}
	return records, nil
}

func checkGroup(name string, g batchGroup, listed map[int64]bool) error {
	if !slices.Contains(groupFiles[name], g.Kind) {
		return fmt.Errorf("%s holds a group of kind %q", name, g.Kind)
	}
	if len(g.Reports) == 0 {
		return fmt.Errorf("%s holds a group with no reports", name)
	}
	for _, r := range g.Reports {
		if r.DocumentKey != g.DocumentKey || r.Anchor != g.Anchor || r.Kind != g.Kind || r.Target != g.Target {
			return fmt.Errorf("%s: report %d differs from its group", name, r.ReportID)
		}
		if !listed[r.ReportID] {
			return fmt.Errorf("%s: report %d is not in the manifest", name, r.ReportID)
		}
		delete(listed, r.ReportID)
	}
	return nil
}

func groupRecord(id string, g batchGroup) Record {
	anchor := g.Anchor
	if !validAnchor(anchor) {
		anchor = ""
	}
	r := Record{Origin: "issue_report", OriginID: id, Kind: g.Kind, Target: Target{Key: g.DocumentKey, Anchor: anchor}}
	switch g.Kind {
	case "wrong_text":
		r.Claim.Code = "reported"
	case "wrong_validity":
		r.Claim.Code = "status_conflict"
	case "broken_citation":
		r.Claim.Code = "wrong_number"
	case "missing_document":
		r.Claim.Code = "missing_document"
		r.Target = Target{Number: g.Target}
	}
	return r
}
