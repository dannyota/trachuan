package discover

import (
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"danny.vn/trachuan/apps/corpus/internal/lookup"
	"danny.vn/trachuan/apps/corpus/internal/source"
	"danny.vn/trachuan/apps/corpus/internal/sourcegate"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/vbpl"
)

type scopeReport struct {
	Version       int             `json:"version"`
	QueryIdentity string          `json:"query_identity"`
	Passes        []lookup.Result `json:"passes"`
	Stable        bool            `json:"stable"`
}
type selectedItem struct {
	ID        string                 `json:"id"`
	Number    *string                `json:"number"`
	Title     *string                `json:"title"`
	IssueDate *string                `json:"issue_date"`
	Issuer    *string                `json:"issuer"`
	AgencyIDs []string               `json:"agency_ids"`
	Rules     []source.SelectionRule `json:"rules"`
	Page      pageReference          `json:"page"`
	RulePages []rulePage             `json:"rule_pages,omitempty"`
}
type rulePage struct {
	Rule source.SelectionRule `json:"rule"`
	Page pageReference        `json:"page"`
}
type pageReference struct {
	Pass   int    `json:"pass"`
	Page   int    `json:"page"`
	SHA256 string `json:"sha256"`
	Path   string `json:"path"`
}
type selectedFile struct {
	Items []selectedItem `json:"items"`
}
type catalogFile struct {
	IDs      []string `json:"ids"`
	Complete bool     `json:"complete"`
}

func (s *Stage) processScope(ctx context.Context, env stage.Env, r stage.Run, it stage.Item, rules source.VBPLSources) (stage.Output, error) {
	out := stage.Output{Sources: []string{"vbpl"}}
	if s.client == nil {
		return out, fmt.Errorf("VBPL client is required")
	}
	named := strings.TrimPrefix(it.Key, "vbpl:named:")
	if it.Key != "vbpl:central" {
		found := false
		for _, entry := range rules.NamedNumbers {
			if it.Key == "vbpl:named:"+entry {
				found = true
				break
			}
		}
		if !found {
			return out, fmt.Errorf("unknown discovery query %q", it.Key)
		}
	}
	art, err := env.Artifacts.PublishContent(ctx, s.Name(), s.Recipe(r.Snapshot), func(dir string) error {
		if it.Key == "vbpl:central" {
			return s.central(ctx, dir, rules)
		}
		entry, err := source.ParseNamedNumber(named)
		if err != nil {
			return fmt.Errorf("parse named entry: %w", err)
		}
		return s.named(ctx, dir, entry.Number)
	})
	if err != nil {
		return out, fmt.Errorf("publish discovery artifact: %w", err)
	}
	out.Artifact = &art
	out.State = "done"
	return out, nil
}
func writeJSON(dir, name string, value any) error {
	b, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode %s: %w", name, err)
	}
	if err = os.WriteFile(filepath.Join(dir, name), b, 0600); err != nil {
		return fmt.Errorf("write %s: %w", name, err)
	}
	return nil
}
func writePage(dir string, pass, page int, raw []byte) (pageReference, error) {
	name := fmt.Sprintf("pages/p%d-%d.json.gz", pass, page)
	if err := os.MkdirAll(filepath.Join(dir, "pages"), 0700); err != nil {
		return pageReference{}, fmt.Errorf("create pages: %w", err)
	}
	f, err := os.Create(filepath.Join(dir, name))
	if err != nil {
		return pageReference{}, fmt.Errorf("create page: %w", err)
	}
	gz := gzip.NewWriter(f)
	_, err = gz.Write(raw)
	if err != nil {
		_ = gz.Close()
		_ = f.Close()
		return pageReference{}, fmt.Errorf("compress page: %w", err)
	}
	if err = gz.Close(); err != nil {
		_ = f.Close()
		return pageReference{}, fmt.Errorf("finish page: %w", err)
	}
	if err = f.Close(); err != nil {
		return pageReference{}, fmt.Errorf("close page: %w", err)
	}
	sum := sha256.Sum256(raw)
	return pageReference{Pass: pass, Page: page, SHA256: fmt.Sprintf("%x", sum), Path: name}, nil
}
func ids(items []lookup.Item) []string {
	out := make([]string, 0, len(items))
	for _, it := range items {
		out = append(out, it.SourceID)
	}
	sort.Strings(out)
	return out
}
func sameIDs(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
func retryLookup(err error) error {
	wrapped := fmt.Errorf("discover VBPL: %w", err)
	if errors.Is(err, sourcegate.ErrSourcePaused) {
		return stage.Retryable(wrapped)
	}
	var f *vbpl.Failure
	if errors.As(err, &f) {
		cause := stage.GapCause{Source: "vbpl", Origin: stage.Origin(f.Origin), Cause: stage.Cause(f.Cause), Endpoint: f.Endpoint}
		if f.HTTPStatus != 0 {
			status := int32(f.HTTPStatus)
			cause.HTTPStatus = &status
		}
		return &stage.RetryableError{Err: wrapped, GapKind: "source_unavailable", Cause: cause}
	}
	var st *vbpl.StatusError
	if errors.As(err, &st) && (st.Code == 429 || st.Code >= 500 && st.Code <= 599) {
		status := int32(st.Code)
		cause := stage.CauseHTTP5xx
		if st.Code == 429 {
			cause = stage.CauseHTTP429
		}
		return &stage.RetryableError{Err: wrapped, GapKind: "source_unavailable", Cause: stage.GapCause{Source: "vbpl", Origin: stage.OriginGateway, Cause: cause, Endpoint: st.Endpoint, HTTPStatus: &status}}
	}
	return stage.Retryable(wrapped)
}
