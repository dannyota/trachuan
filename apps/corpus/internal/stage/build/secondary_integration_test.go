//go:build integration

package build

import (
	"context"
	"crypto/sha256"
	"fmt"
	"testing"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/stage/membership"
)

func secondaryFetchFixture(t *testing.T, f *loadFixture, key string) provenanceFile {
	t.Helper()
	url := "https://example.test/document"
	html := []byte("detail")
	metadata := []byte(`{"version":1,"source_id":"1","source_url":"https://example.test/document","occurrences":[],"attachments":[]}`)
	h := func(b []byte) string { return fmt.Sprintf("%x", sha256.Sum256(b)) }
	doc := map[string]any{"version": 2, "document": map[string]any{"key": key, "country": "vn", "source": "chinhphu", "source_id": "1", "number": "SECONDARY-1", "title": "Secondary title", "doc_type": "Decree", "issuer": "Government", "raw_issue_date": "2024-01-02", "source_url": url, "identity_evidence": []any{map[string]any{"path": "detail.html", "sha256": h(html)}, map[string]any{"path": "metadata.json", "sha256": h(metadata)}}}}
	body := []byte("secondary file")
	hash := h(body)
	path := "files/" + hash
	f.addArtifact(t, "fetch", key, map[string]any{"document.json": doc, "files.json": map[string]any{"version": 2, "files": []any{map[string]any{"source": "chinhphu", "name": "secondary.pdf", "sha256": hash, "size": len(body), "path": path, "url": url, "role": "main"}}}, "metadata.json": metadata, "detail.html": html, path: body})
	role := "main"
	return provenanceFile{DocumentKey: key, Source: "chinhphu", Name: "secondary.pdf", SHA256: hash, Size: int64(len(body)), Path: path, URL: &url, Role: &role}
}

func TestLoad_SecondaryMetadataOnly(t *testing.T) {
	f := fixture(t)
	key := "vn:chinhphu:1"
	file := secondaryFetchFixture(t, f, key)
	inv := f.inventory(t)
	inv.Provenance = provenance{Version: 1, Files: []provenanceFile{file}}
	inv.Version = 2
	inv.Membership.Entries = []membership.Entry{{DocumentKey: key, Disposition: "active", Class: "secondary", Source: "chinhphu"}}
	counts, err := Load(context.Background(), f.corpus, inv, f.artifacts, f.recipes, nil, nil, f.asOf())
	if err != nil {
		t.Fatal(err)
	}
	var number, title, class, source, state string
	err = f.corpus.QueryRow(context.Background(), `SELECT number,title,class,source,content_state FROM document WHERE key=$1`, key).Scan(&number, &title, &class, &source, &state)
	if err != nil || number != "SECONDARY-1" || title != "Secondary title" || class != "secondary" || source != "chinhphu" || state != "no_tree" || counts.Provisions != 0 || counts.Chunks != 0 || counts.Relations != 0 || counts.SourceFiles != 1 {
		t.Fatalf("document %q %q %q %q %q counts=%+v error=%v", number, title, class, source, state, counts, err)
	}
	var gaps int
	if err = f.corpus.QueryRow(context.Background(), `SELECT count(*) FROM coverage_gap WHERE document_key=$1 AND kind='no_tree'`, key).Scan(&gaps); err != nil || gaps != 1 {
		t.Fatalf("no_tree gaps=%d error=%v", gaps, err)
	}
}

func TestLoad_ProvenanceFromSnapshot(t *testing.T) {
	f := fixture(t)
	key := "vn:vbpl:1"
	content := []byte("source file")
	hash := fmt.Sprintf("%x", sha256.Sum256(content))
	path := "files/" + hash
	f.addArtifact(t, "fetch", key, map[string]any{"detail.json": map[string]any{"data": map[string]any{"id": "1"}}, "files.json": []any{map[string]any{"name": "law.pdf", "sha256": hash, "size": len(content)}}, path: content})
	inv := f.inventory(t)
	inv.Version = 2
	inv.Membership.Entries = []membership.Entry{{DocumentKey: key, Disposition: "active", Class: "primary", Source: "vbpl"}}
	url, role := "https://example.test/source", "main"
	when := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	inv.Provenance = provenance{Version: 1, Files: []provenanceFile{{DocumentKey: key, Source: "vbpl", Name: "law.pdf", SHA256: hash, Size: int64(len(content)), Path: path, URL: &url, Role: &role, FetchedAt: &when}}}
	if _, err := Load(context.Background(), f.corpus, inv, f.artifacts, f.recipes, nil, nil, f.asOf()); err != nil {
		t.Fatal(err)
	}
	var actualURL, actualRole string
	var actualTime time.Time
	if err := f.corpus.QueryRow(context.Background(), `SELECT url,role,fetched_at FROM source_file`).Scan(&actualURL, &actualRole, &actualTime); err != nil || actualURL != url || actualRole != role || !actualTime.Equal(when) {
		t.Fatalf("provenance %q %q %v error=%v", actualURL, actualRole, actualTime, err)
	}
}

func TestRelations_ReplacedSecondaryNeverResolvesNumber(t *testing.T) {
	f := fixture(t)
	secondary := "vn:chinhphu:1"
	_ = secondaryFetchFixture(t, f, secondary)
	source := "vn:vbpl:2"
	art := f.fetch(t, source)
	f.addArtifact(t, "relate", source, map[string]any{"relations.json": []any{map[string]any{"ordinal": 0, "kind": "references", "target_number": "SECONDARY-1"}}}, f.relateInput(t, source, art.SHA256))
	inv := f.inventory(t)
	inv.Version = 2
	replacement := source
	inv.Membership.Entries = []membership.Entry{{DocumentKey: source, Disposition: "active", Class: "primary", Source: "vbpl"}, {DocumentKey: secondary, Disposition: "source_replaced", Class: "secondary", Source: "chinhphu", ReplacementKey: &replacement}}
	if _, err := Load(context.Background(), f.corpus, inv, f.artifacts, f.recipes, nil, nil, f.asOf()); err != nil {
		t.Fatal(err)
	}
	var key *string
	var id *int64
	if err := f.corpus.QueryRow(context.Background(), `SELECT target_key,target_document_id FROM relation`).Scan(&key, &id); err != nil || key != nil || id != nil {
		t.Fatalf("target=%v id=%v error=%v", key, id, err)
	}
}

func TestLoad_LegacyRunClassPrimary(t *testing.T) {
	f := fixture(t)
	f.fetch(t, "vn:vbpl:1")
	if _, err := f.load(t); err != nil {
		t.Fatal(err)
	}
	var class, source string
	if err := f.corpus.QueryRow(context.Background(), `SELECT class,source FROM document`).Scan(&class, &source); err != nil || class != "primary" || source != "vbpl" {
		t.Fatalf("class=%q source=%q error=%v", class, source, err)
	}
}
