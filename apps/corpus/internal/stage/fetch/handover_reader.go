package fetch

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/chinhphu"
	"danny.vn/trachuan/apps/corpus/internal/source"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/store"
	"danny.vn/trachuan/internal/retrieval"
)

func verifiedHandOverFiles(env stage.Env, item store.ListStageItemArtifactsRow, names ...string) (string, []string, error) {
	if item.Path == nil || item.Sha256 == nil {
		return "", nil, fmt.Errorf("missing artifact for %s", item.ItemKey)
	}
	dir := env.Artifacts.Abs(*item.Path)
	hash, err := artifact.VerifyFiles(dir, names...)
	if err != nil {
		return "", nil, fmt.Errorf("verify %s: %w", item.ItemKey, err)
	}
	if hash != *item.Sha256 {
		return "", nil, fmt.Errorf("artifact hash mismatch for %s", item.ItemKey)
	}
	refs := make([]string, 0, len(names))
	for _, name := range names {
		refs = append(refs, hash+"/"+name)
	}
	return dir, refs, nil
}
func readHandOverJSON(dir, name string, target any) error {
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		return fmt.Errorf("read %s: %w", name, err)
	}
	if err = json.Unmarshal(data, target); err != nil {
		return fmt.Errorf("decode %s: %w", name, err)
	}
	return nil
}
func (*Stage) readVBPLHandOver(_ context.Context, env stage.Env, item store.ListStageItemArtifactsRow) (handOverIdentity, []string, error) {
	dir, refs, err := verifiedHandOverFiles(env, item, "detail.json")
	if err != nil {
		return handOverIdentity{}, nil, err
	}
	var payload struct {
		Data struct{ ID, DocNum, AgencyName, IssueDate string } `json:"data"`
	}
	if err = readHandOverJSON(dir, "detail.json", &payload); err != nil {
		return handOverIdentity{}, nil, err
	}
	if "vn:vbpl:"+payload.Data.ID != item.ItemKey {
		return handOverIdentity{}, nil, fmt.Errorf("VBPL detail key mismatch for %s", item.ItemKey)
	}
	return normalizedIdentity(payload.Data.DocNum, payload.Data.AgencyName, payload.Data.IssueDate), refs, nil
}
func (s *Stage) readSecondaryHandOver(_ context.Context, env stage.Env, item store.ListStageItemArtifactsRow) (handOverIdentity, []string, error) {
	if s.chinhphu == nil {
		return handOverIdentity{}, nil, fmt.Errorf("chinhphu client required for %s", item.ItemKey)
	}
	dir, refs, err := verifiedHandOverFiles(env, item, "document.json", "metadata.json", "detail.html")
	if err != nil {
		return handOverIdentity{}, nil, err
	}
	var envelope secondaryDocumentEnvelope
	var metadata secondaryMetadata
	if err = readHandOverJSON(dir, "document.json", &envelope); err != nil {
		return handOverIdentity{}, nil, err
	}
	if err = readHandOverJSON(dir, "metadata.json", &metadata); err != nil {
		return handOverIdentity{}, nil, err
	}
	doc := envelope.Document
	if envelope.Version != 2 || metadata.Version != 1 || doc.Key != item.ItemKey || doc.Source != "chinhphu" || doc.Country != "vn" || doc.Key != "vn:chinhphu:"+doc.SourceID || metadata.SourceID != doc.SourceID || doc.SourceURL == nil || *doc.SourceURL != metadata.SourceURL {
		return handOverIdentity{}, nil, fmt.Errorf("invalid secondary bundle for %s", item.ItemKey)
	}
	html, err := os.ReadFile(filepath.Join(dir, "detail.html"))
	if err != nil {
		return handOverIdentity{}, nil, fmt.Errorf("read detail: %w", err)
	}
	meta, err := os.ReadFile(filepath.Join(dir, "metadata.json"))
	if err != nil {
		return handOverIdentity{}, nil, fmt.Errorf("read metadata: %w", err)
	}
	if len(doc.IdentityEvidence) != 2 || doc.IdentityEvidence[0].Path != "detail.html" || doc.IdentityEvidence[0].SHA256 != hashBytes(html) || doc.IdentityEvidence[1].Path != "metadata.json" || doc.IdentityEvidence[1].SHA256 != hashBytes(meta) {
		return handOverIdentity{}, refs, nil
	}
	parsed, err := s.chinhphu.ParseDetail(html, *doc.SourceURL)
	if err != nil {
		return handOverIdentity{}, refs, nil
	}
	fields := make([]chinhphu.FieldOccurrence, 0, len(metadata.Occurrences))
	for _, f := range metadata.Occurrences {
		value := ""
		if f.Value != nil {
			value = *f.Value
		}
		fields = append(fields, chinhphu.FieldOccurrence{Name: f.Name, Value: value, Cells: f.Cells, Ambiguous: f.Ambiguous})
	}
	return checkedSecondaryIdentity(doc, parsed.Occurrences, fields), refs, nil
}
func checkedSecondaryIdentity(doc secondaryDocument, parsed, metadata []chinhphu.FieldOccurrence) handOverIdentity {
	check := func(name, value string, normalize func(string) string) string {
		var a, b []chinhphu.FieldOccurrence
		for _, field := range parsed {
			if strings.TrimSpace(field.Name) == name {
				a = append(a, field)
			}
		}
		for _, field := range metadata {
			if strings.TrimSpace(field.Name) == name {
				b = append(b, field)
			}
		}
		if len(a) != len(b) {
			return ""
		}
		for i := range a {
			if a[i].Name != b[i].Name || a[i].Value != b[i].Value || a[i].Ambiguous != b[i].Ambiguous || !slices.Equal(a[i].Cells, b[i].Cells) {
				return ""
			}
		}
		raw, ok := secondaryField(parsed, name, normalize)
		if !ok || normalize(raw) == "" || normalize(raw) != normalize(value) {
			return ""
		}
		return normalize(raw)
	}
	return handOverIdentity{number: check("Số ký hiệu", doc.Number, retrieval.NormalizeNumber), issuer: check("Cơ quan ban hành", doc.Issuer, source.NormalizeIssuer), date: check("Ngày ban hành", doc.IssueDate, normalizeDate)}
}
