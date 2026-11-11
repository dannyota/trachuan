package fetch

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"danny.vn/trachuan/apps/corpus/internal/stage"
)

// Document is the source-neutral metadata of a verified fetch bundle.
type Document struct {
	Key, Number, Title, DocType, Issuer, IssueDate, EffFrom, EffTo string
	StatusCode, StatusName                                         string
	IsConsolidated                                                 bool
	SourceURL                                                      *string
	Files                                                          []DocumentFile
}

// DocumentFile identifies a file in a fetch bundle.
type DocumentFile struct {
	Name, SHA256 string
	Size         int64
}

// ReadDocument decodes a fetch bundle according to its document key.
// The caller verifies the artifact manifest and its path before calling.
func ReadDocument(dir, key string) (Document, error) {
	parts := strings.SplitN(key, ":", 3)
	if len(parts) != 3 || parts[0] != "vn" || parts[2] == "" {
		return Document{}, fmt.Errorf("invalid document key")
	}
	if parts[1] == "chinhphu" {
		d, files, err := readSecondary(dir)
		if err != nil {
			return Document{}, fmt.Errorf("read secondary bundle: %w", err)
		}
		if d.Key != key {
			return Document{}, fmt.Errorf("fetch identity mismatch")
		}
		out := Document{Key: d.Key, Number: d.Number, Title: d.Title, DocType: d.DocType, Issuer: d.Issuer, IssueDate: d.IssueDate, EffFrom: d.EffFrom, EffTo: d.EffTo, StatusCode: d.StatusCode, StatusName: d.StatusName, IsConsolidated: d.IsConsolidated, SourceURL: d.SourceURL}
		for _, f := range files {
			out.Files = append(out.Files, DocumentFile{Name: f.Name, SHA256: f.SHA256, Size: f.Size})
		}
		return out, nil
	}
	if parts[1] != "vbpl" {
		return Document{}, fmt.Errorf("unsupported fetch source %q", parts[1])
	}
	var detail detailEnvelope
	var files []file
	for _, entry := range []struct {
		name string
		dst  any
	}{{"detail.json", &detail}, {"files.json", &files}} {
		data, err := os.ReadFile(filepath.Join(dir, entry.name))
		if err != nil {
			return Document{}, fmt.Errorf("read %s: %w", entry.name, err)
		}
		if err = json.Unmarshal(data, entry.dst); err != nil {
			return Document{}, fmt.Errorf("decode %s: %w", entry.name, err)
		}
	}
	d := detail.Data
	if "vn:vbpl:"+d.ID != key {
		return Document{}, fmt.Errorf("fetch identity mismatch")
	}
	out := Document{Key: key, Number: d.DocNum, Title: d.Title, DocType: d.DocType.Name, Issuer: d.AgencyName, IssueDate: d.IssueDate, EffFrom: d.EffFrom, EffTo: d.EffTo, StatusCode: d.EffStatus.Code, StatusName: d.EffStatus.Name, IsConsolidated: d.IsConsolidatedDocument}
	for _, f := range files {
		out.Files = append(out.Files, DocumentFile(f))
	}
	return out, nil
}

// ExtractInputHash binds the fetch manifest and selected text source.
func ExtractInputHash(fetchHash, source string) string {
	if source == "source_tree" {
		return fetchHash
	}
	if source == "none" || source == "ocr" {
		return stage.RecipeHash("extract-input", "1", fetchHash, source, "absent")
	}
	return stage.RecipeHash("extract-input", "1", fetchHash, source)
}

// CheckExtractLineage checks an extract artifact against its verified fetch hash.
// The caller verifies the extract manifest before calling this function.
func CheckExtractLineage(dir, fetchHash, inputHash string) error {
	data, err := os.ReadFile(filepath.Join(dir, "text.json"))
	if errors.Is(err, os.ErrNotExist) {
		if inputHash == fetchHash {
			return nil
		}
		return fmt.Errorf("artifact input hash mismatch")
	}
	if err != nil {
		return fmt.Errorf("read extract text record: %w", err)
	}
	var record struct {
		TextSource string `json:"text_source"`
	}
	if err = json.Unmarshal(data, &record); err != nil {
		return fmt.Errorf("decode extract text record: %w", err)
	}
	switch record.TextSource {
	case "none", "html_transcription", "source_tree":
	default:
		return fmt.Errorf("invalid text source %q", record.TextSource)
	}
	if ExtractInputHash(fetchHash, record.TextSource) != inputHash {
		return fmt.Errorf("artifact input hash mismatch")
	}
	return nil
}
