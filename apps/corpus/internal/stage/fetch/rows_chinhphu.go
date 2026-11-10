package fetch

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"danny.vn/trachuan/apps/corpus/internal/store"
	"github.com/jackc/pgx/v5"
)

func readSecondary(dir string) (secondaryDocument, []secondaryFile, error) {
	var envelope secondaryDocumentEnvelope
	var files secondaryFiles
	var metadata secondaryMetadata
	for _, name := range []string{"detail.json", "tree.json", "diagram.json"} {
		_, err := os.Stat(filepath.Join(dir, name))
		if err == nil {
			return secondaryDocument{}, nil, fmt.Errorf("mixed fetch formats")
		}
		if !os.IsNotExist(err) {
			return secondaryDocument{}, nil, fmt.Errorf("stat %s: %w", name, err)
		}
	}
	for _, entry := range []struct {
		name string
		dst  any
	}{{"document.json", &envelope}, {"files.json", &files}, {"metadata.json", &metadata}} {
		b, err := os.ReadFile(filepath.Join(dir, entry.name))
		if err != nil {
			return secondaryDocument{}, nil, fmt.Errorf("read %s: %w", entry.name, err)
		}
		if err = json.Unmarshal(b, entry.dst); err != nil {
			return secondaryDocument{}, nil, fmt.Errorf("decode %s: %w", entry.name, err)
		}
	}
	d := envelope.Document
	if envelope.Version != 2 || files.Version != 2 || metadata.Version != 1 || files.Files == nil || metadata.Occurrences == nil || metadata.Attachments == nil {
		return d, nil, fmt.Errorf("unknown fetch bundle version")
	}
	if d.Country != "vn" || d.Source != "chinhphu" || d.SourceID == "" || d.Key != d.Country+":"+d.Source+":"+d.SourceID || metadata.SourceID != d.SourceID || d.SourceURL == nil || *d.SourceURL != metadata.SourceURL {
		return d, nil, fmt.Errorf("inconsistent secondary identity")
	}
	if len(d.IdentityEvidence) != 2 {
		return d, nil, fmt.Errorf("invalid identity evidence")
	}
	for i, ref := range d.IdentityEvidence {
		wanted := []string{"detail.html", "metadata.json"}[i]
		if ref.Path != wanted {
			return d, nil, fmt.Errorf("invalid identity reference")
		}
		b, err := os.ReadFile(filepath.Join(dir, ref.Path))
		if err != nil {
			return d, nil, fmt.Errorf("read identity evidence: %w", err)
		}
		if hashBytes(b) != ref.SHA256 {
			return d, nil, fmt.Errorf("identity evidence hash mismatch")
		}
	}
	for _, f := range files.Files {
		_, hashErr := hex.DecodeString(f.SHA256)
		if f.Source != d.Source || f.URL == nil || (f.Role != "main" && f.Role != "appendix" && f.Role != "scan" && f.Role != "unknown") || len(f.SHA256) != 64 || hashErr != nil || f.Path != "files/"+f.SHA256 || strings.ToLower(f.SHA256) != f.SHA256 {
			return d, nil, fmt.Errorf("invalid secondary file")
		}
		b, err := os.ReadFile(filepath.Join(dir, f.Path))
		if err != nil {
			return d, nil, fmt.Errorf("read secondary file: %w", err)
		}
		if hashBytes(b) != f.SHA256 || int64(len(b)) != f.Size {
			return d, nil, fmt.Errorf("file hash mismatch")
		}
	}
	return d, files.Files, nil
}
func chinhphuRows(dir string) (func(context.Context, pgx.Tx, int64) error, error) {
	d, files, err := readSecondary(dir)
	if err != nil {
		return nil, err
	}
	evidence, err := json.Marshal(d.IdentityEvidence)
	if err != nil {
		return nil, fmt.Errorf("encode identity evidence: %w", err)
	}
	sort.Slice(files, func(i, j int) bool {
		if files[i].Name != files[j].Name {
			return files[i].Name < files[j].Name
		}
		if files[i].SHA256 != files[j].SHA256 {
			return files[i].SHA256 < files[j].SHA256
		}
		return *files[i].URL < *files[j].URL
	})
	return func(ctx context.Context, tx pgx.Tx, id int64) error {
		q := store.New(tx)
		if e := q.InsertDocument(ctx, store.InsertDocumentParams{ArtifactID: id, Key: d.Key, Country: d.Country, Source: d.Source, SourceID: d.SourceID, Number: d.Number, Title: d.Title, DocType: d.DocType, Issuer: d.Issuer, IssueDate: d.IssueDate, EffFrom: d.EffFrom, EffTo: d.EffTo, StatusCode: d.StatusCode, StatusName: d.StatusName, IsConsolidated: d.IsConsolidated, HasContent: d.HasContent, SourceUrl: d.SourceURL, IdentityEvidence: evidence}); e != nil {
			return fmt.Errorf("insert document: %w", e)
		}
		for i, f := range files {
			if i > 0 && files[i-1].Name == f.Name && files[i-1].SHA256 == f.SHA256 {
				continue
			}
			if e := q.InsertFetchedFile(ctx, store.InsertFetchedFileParams{ArtifactID: id, Source: f.Source, Name: f.Name, Sha256: f.SHA256, Size: f.Size, Path: f.Path, Url: f.URL, Role: f.Role}); e != nil {
				return fmt.Errorf("insert fetched file: %w", e)
			}
		}
		return nil
	}, nil
}
