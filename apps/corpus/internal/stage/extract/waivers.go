package extract

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/ocrstore"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"gopkg.in/yaml.v3"
)

type WaiverItem struct {
	Page    int    `yaml:"page" json:"page"`
	Kind    string `yaml:"kind" json:"kind"`
	OCR     string `yaml:"ocr" json:"ocr"`
	PDF     string `yaml:"pdf" json:"pdf"`
	Reading string `yaml:"reading" json:"reading"`
}

type Waiver struct {
	Key         string       `yaml:"key" json:"key"`
	PDFSHA256   string       `yaml:"pdf_sha256" json:"pdf_sha256"`
	CheckSHA256 string       `yaml:"check_sha256" json:"check_sha256"`
	Items       []WaiverItem `yaml:"items" json:"items"`
	Reason      string       `yaml:"reason" json:"reason"`
	Reviewed    string       `yaml:"reviewed" json:"reviewed"`
}

type Waivers struct {
	entries []Waiver
	hash    string
}

func ReadWaivers(path string) (Waivers, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		data = []byte("[]")
	} else if err != nil {
		return Waivers{}, fmt.Errorf("read OCR waivers: %w", err)
	}
	entries, hash, err := parseWaivers(data)
	if err != nil {
		return Waivers{}, err
	}
	return Waivers{entries: entries, hash: hash}, nil
}

func parseWaivers(data []byte) ([]Waiver, string, error) {
	var entries []Waiver
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&entries); err != nil {
		return nil, "", fmt.Errorf("parse OCR waivers: %w", err)
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		if err != nil {
			return nil, "", fmt.Errorf("parse OCR waivers: %w", err)
		}
		return nil, "", fmt.Errorf("multiple waiver documents")
	}
	if entries == nil {
		return nil, "", fmt.Errorf("waivers must be a list")
	}
	seen := map[string]bool{}
	for _, w := range entries {
		if strings.TrimSpace(w.Key) == "" || seen[w.Key] || !validWaiverHash(w.PDFSHA256) || !validWaiverHash(w.CheckSHA256) || len(w.Items) == 0 || strings.TrimSpace(w.Reason) == "" {
			return nil, "", fmt.Errorf("invalid OCR waiver %q", w.Key)
		}
		if _, err := time.Parse("2006-01-02", w.Reviewed); err != nil {
			return nil, "", fmt.Errorf("invalid waiver review date: %w", err)
		}
		seen[w.Key] = true
		for _, item := range w.Items {
			if item.Page < 1 || (item.Kind != "article_label" && item.Kind != "document_number") || strings.TrimSpace(item.OCR) == "" || strings.TrimSpace(item.PDF) == "" || strings.TrimSpace(item.Reading) == "" {
				return nil, "", fmt.Errorf("invalid OCR waiver item %q", w.Key)
			}
		}
	}
	slices.SortFunc(entries, func(a, b Waiver) int { return strings.Compare(a.Key, b.Key) })
	canonical, err := json.Marshal(entries)
	if err != nil {
		return nil, "", fmt.Errorf("encode OCR waivers: %w", err)
	}
	sum := sha256.Sum256(canonical)
	return entries, hex.EncodeToString(sum[:]), nil
}
func validWaiverHash(s string) bool {
	if len(s) != 64 || strings.ToLower(s) != s {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}
func (w Waivers) find(key string) *Waiver {
	for i := range w.entries {
		if w.entries[i].Key == key {
			return &w.entries[i]
		}
	}
	return nil
}
func matchWaiver(w *Waiver, pdf string, entry ocrstore.Entry, result ocrstore.CheckResult) (bool, bool, error) {
	if w == nil {
		return false, false, nil
	}
	if entry.Check == nil || pdf != w.PDFSHA256 || entry.Check.ResultSha256 != w.CheckSHA256 {
		return false, true, nil
	}
	counts := map[string]int{}
	for _, item := range w.Items {
		counts[waiverItemKey(item.Page, item.Kind, item.OCR, item.PDF)]++
	}
	n := 0
	for _, item := range result.Items {
		if item.Kind == "article_label" || item.Kind == "document_number" {
			n++
			k := waiverItemKey(item.Page, item.Kind, item.OCR, item.PDF)
			counts[k]--
			if counts[k] < 0 {
				return false, false, fmt.Errorf("OCR waiver %q items differ from check", w.Key)
			}
		}
	}
	if n != len(w.Items) || n == 0 {
		return false, false, fmt.Errorf("OCR waiver %q items differ from check", w.Key)
	}
	return true, false, nil
}

func waiverItemKey(page int, kind, ocr, pdf string) string {
	return fmt.Sprintf("%d\x00%s\x00%s\x00%s", page, kind, ocr, pdf)
}

func (w Waivers) validateSelections(seen map[string]bool) error {
	for _, entry := range w.entries {
		if !seen[entry.Key] {
			return fmt.Errorf("OCR waiver %q has no OCR selection", entry.Key)
		}
	}
	return nil
}

func annotateWaiver(w *Waiver, selected selection, record *textRecord, gaps []stage.Gap) []stage.Gap {
	if w == nil {
		return gaps
	}
	if record != nil && selected.source == "ocr" && selected.entry != nil && selected.entry.Check != nil && selected.hash == w.PDFSHA256 && selected.entry.Check.ResultSha256 == w.CheckSHA256 {
		record.Waiver = &waiverRecord{Key: w.Key, CheckSHA256: w.CheckSHA256}
		return gaps
	}
	return append(gaps, stage.Gap{Kind: "ocr_waiver_stale", Detail: "waiver did not apply to the frozen OCR selection: " + w.Key})
}
