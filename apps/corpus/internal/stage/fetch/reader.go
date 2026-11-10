package fetch

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
