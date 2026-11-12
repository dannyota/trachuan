package fetch

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/chinhphu"
	"danny.vn/trachuan/apps/corpus/internal/source"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/internal/retrieval"
)

func (s *Stage) writeChinhphu(ctx context.Context, dir, id string, detail chinhphu.Detail, html []byte, observations *[]stage.FetchObservation) error {
	return s.writeChinhphuEvidence(ctx, dir, id, detail, html, observations, nil, nil, time.Now)
}

func (s *Stage) writeChinhphuEvidence(ctx context.Context, dir, id string, detail chinhphu.Detail, html []byte, observations *[]stage.FetchObservation, counts *readDecision, q hashStore, now func() time.Time) error {
	meta := secondaryMetadata{Version: 1, SourceID: id, SourceURL: detail.URL, Occurrences: []secondaryOccurrence{}, Attachments: []secondaryAttachment{}}
	for _, field := range detail.Occurrences {
		cells := append([]string{}, field.Cells...)
		var value *string
		if !field.Ambiguous && field.Value != "" {
			v := field.Value
			value = &v
		}
		meta.Occurrences = append(meta.Occurrences, secondaryOccurrence{Name: field.Name, Value: value, Cells: cells, Ambiguous: field.Ambiguous})
	}
	for _, a := range detail.Attachments {
		meta.Attachments = append(meta.Attachments, secondaryAttachment{Name: a.Name, Label: a.Label, URL: a.URL})
	}
	sort.Slice(meta.Attachments, func(i, j int) bool {
		a, b := meta.Attachments[i], meta.Attachments[j]
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		if a.URL != b.URL {
			return a.URL < b.URL
		}
		return a.Label < b.Label
	})
	meta.Attachments = dedupAttachments(meta.Attachments)
	if conflictingFields(detail.Occurrences, "Số ký hiệu", retrieval.NormalizeNumber) || conflictingFields(detail.Occurrences, "Cơ quan ban hành", source.NormalizeIssuer) || conflictingFields(detail.Occurrences, "Ngày ban hành", normalizeDate) {
		return stage.SourceData(fmt.Errorf("conflicting secondary identity"))
	}
	number, _ := secondaryField(detail.Occurrences, "Số ký hiệu", retrieval.NormalizeNumber)
	issuer, _ := secondaryField(detail.Occurrences, "Cơ quan ban hành", source.NormalizeIssuer)
	date := rawDate(detail.Occurrences, "Ngày ban hành")
	title, _ := secondaryField(detail.Occurrences, "Trích yếu", strings.TrimSpace)
	docType, _ := secondaryField(detail.Occurrences, "Loại văn bản", strings.TrimSpace)
	effective := rawDate(detail.Occurrences, "Ngày có hiệu lực")
	endDate := rawDate(detail.Occurrences, "Ngày hết hiệu lực")
	metaBytes, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("encode metadata: %w", err)
	}
	doc := secondaryDocument{Key: "vn:chinhphu:" + id, Country: "vn", Source: "chinhphu", SourceID: id, Number: number, Title: title, DocType: docType, Issuer: issuer, IssueDate: date, EffFrom: effective, EffTo: endDate, SourceURL: &detail.URL, IdentityEvidence: []evidenceRef{{Path: "detail.html", SHA256: hashBytes(html)}, {Path: "metadata.json", SHA256: hashBytes(metaBytes)}}}
	files := secondaryFiles{Version: 2, Files: []secondaryFile{}}
	report := secondaryReport{Version: 1, Missing: []secondaryOutcome{}, TooLarge: []secondaryOutcome{}}
	if err = os.Mkdir(filepath.Join(dir, "files"), 0700); err != nil {
		return fmt.Errorf("create files: %w", err)
	}
	pdfCtx, cancelPDFs := context.WithTimeout(ctx, pdfDownloadLimit)
	defer cancelPDFs()
	budget := newPDFBudget(now)
	seen := map[secondaryOutcome]bool{}
	for _, a := range meta.Attachments {
		outcome := secondaryOutcome{Name: a.Name, URL: a.URL}
		if seen[outcome] {
			continue
		}
		seen[outcome] = true
		var info chinhphu.FileInfo
		var e error
		if errors.Is(pdfCtx.Err(), context.DeadlineExceeded) {
			e = errPDFDeadline
		} else {
			info, e = budget.download(pdfCtx, s, dir, a.URL)
		}
		if errors.Is(e, context.DeadlineExceeded) && errors.Is(pdfCtx.Err(), context.DeadlineExceeded) {
			e = errPDFDeadline
		}
		if e != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if missingFailure(e) || errors.Is(e, errPDFDeadline) {
				cause := failureCause(e)
				if errors.Is(e, errPDFDeadline) {
					cause = "deadline"
				}
				if counts != nil {
					counts.count("failure_" + cause)
				}
				slog.Warn("attachment missing", "document_key", "vn:chinhphu:"+id, "file_name", a.Name, "cause", cause)
				report.Missing = append(report.Missing, outcome)
				continue
			}
			if errors.Is(e, chinhphu.ErrTooLarge) {
				report.TooLarge = append(report.TooLarge, outcome)
				continue
			}
			return fmt.Errorf("download attachment: %w", e)
		}
		if e = writeHashes(ctx, q, info.Hashes); e != nil {
			return e
		}
		u := a.URL
		name := a.Name
		files.Files = append(files.Files, secondaryFile{Source: "chinhphu", Name: name, SHA256: info.SHA256, Size: info.Size, Path: "files/" + info.SHA256, URL: &u, Role: "unknown"})
		key, _ := json.Marshal([3]any{"attachment", name, u})
		fetched := info.FetchedAt.UTC().Truncate(time.Microsecond)
		*observations = append(*observations, stage.FetchObservation{ResourceKey: string(key), Source: "chinhphu", Name: &name, URL: &u, Path: "files/" + info.SHA256, ContentSHA256: info.SHA256, Size: info.Size, Acquisition: "network", FetchedAt: &fetched})
	}
	for _, entry := range []struct {
		name  string
		value any
	}{{"metadata.json", json.RawMessage(metaBytes)}, {"document.json", secondaryDocumentEnvelope{Version: 2, Document: doc}}, {"files.json", files}, {"report.json", report}} {
		data, e := json.Marshal(entry.value)
		if e != nil {
			return fmt.Errorf("encode %s: %w", entry.name, e)
		}
		if e = os.WriteFile(filepath.Join(dir, entry.name), data, 0600); e != nil {
			return fmt.Errorf("write %s: %w", entry.name, e)
		}
	}
	if err = os.WriteFile(filepath.Join(dir, "detail.html"), html, 0600); err != nil {
		return fmt.Errorf("write detail: %w", err)
	}
	return nil
}
func hashBytes(b []byte) string { return fmt.Sprintf("%x", sha256.Sum256(b)) }
func dedupAttachments(in []secondaryAttachment) []secondaryAttachment {
	out := make([]secondaryAttachment, 0, len(in))
	for _, a := range in {
		if len(out) == 0 || out[len(out)-1] != a {
			out = append(out, a)
		}
	}
	return out
}
func secondaryField(fields []chinhphu.FieldOccurrence, name string, normalize func(string) string) (string, bool) {
	value, norm := "", ""
	found := false
	for _, f := range fields {
		if strings.TrimSpace(f.Name) != name {
			continue
		}
		if f.Ambiguous {
			return "", false
		}
		n := normalize(f.Value)
		if n == "" || found && n != norm {
			return "", false
		}
		if !found {
			value = f.Value
			norm = n
			found = true
		}
	}
	return value, found
}
func conflictingFields(fields []chinhphu.FieldOccurrence, name string, normalize func(string) string) bool {
	previous := ""
	for _, field := range fields {
		if strings.TrimSpace(field.Name) != name {
			continue
		}
		if field.Ambiguous {
			return true
		}
		value := normalize(field.Value)
		if value != "" && previous != "" && value != previous {
			return true
		}
		if value != "" {
			previous = value
		}
	}
	return false
}
func normalizeDate(v string) string {
	for _, layout := range []string{"2006-01-02", "2006-01-02T15:04:05", "02-01-2006", "02/01/2006"} {
		if d, e := time.Parse(layout, strings.TrimSpace(v)); e == nil {
			return d.Format("2006-01-02")
		}
	}
	return ""
}
func rawDate(fields []chinhphu.FieldOccurrence, name string) string {
	value, ok := secondaryField(fields, name, normalizeDate)
	if !ok {
		value, _ = secondaryField(fields, name, strings.TrimSpace)
	}
	return value
}
func (s *Stage) downloadChinhphu(ctx context.Context, dir, url string) (chinhphu.FileInfo, error) {
	f, err := os.CreateTemp(filepath.Join(dir, "files"), ".download-")
	if err != nil {
		return chinhphu.FileInfo{}, fmt.Errorf("create download: %w", err)
	}
	defer func() { _ = os.Remove(f.Name()) }()
	info, downloadErr := s.chinhphu.Download(ctx, url, f, s.fileLimit)
	closeErr := f.Close()
	if downloadErr != nil {
		return chinhphu.FileInfo{}, fmt.Errorf("download PDF: %w", downloadErr)
	}
	if closeErr != nil {
		return chinhphu.FileInfo{}, fmt.Errorf("close PDF: %w", closeErr)
	}
	if err = os.Rename(f.Name(), filepath.Join(dir, "files", info.SHA256)); err != nil {
		return chinhphu.FileInfo{}, fmt.Errorf("store PDF: %w", err)
	}
	return info, nil
}
