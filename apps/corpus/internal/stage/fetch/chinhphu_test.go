package fetch

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/chinhphu"
	"danny.vn/trachuan/apps/corpus/internal/stage"
)

type chinhphuOption interface{ WithChinhphu(*chinhphu.Client) *Stage }

func secondaryFixture(t *testing.T, missing bool, token *string, fileStatus ...int) (*Stage, stage.Env) {
	t.Helper()
	raw, err := os.ReadFile("../../chinhphu/testdata/detail-218029.html")
	if err != nil {
		t.Fatal(err)
	}
	var base string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".pdf") {
			if len(fileStatus) > 0 && fileStatus[0] != 200 {
				w.WriteHeader(fileStatus[0])
				return
			}
			if len(fileStatus) > 0 {
				w.Header().Set("Content-Type", "application/pdf")
				_, _ = w.Write([]byte("invalid"))
				return
			}
			if missing {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "application/pdf")
			_, _ = w.Write([]byte("%PDF-test"))
			return
		}
		body := bytes.ReplaceAll(raw, []byte("https://datafiles.chinhphu.vn/cpp/files/"), []byte(base+"/cpp/files/"))
		body = bytes.ReplaceAll(body, []byte("REDACTED-__VIEWSTATE"), []byte(*token))
		_, _ = w.Write(body)
	}))
	base = server.URL
	t.Cleanup(server.Close)
	client := chinhphu.New(chinhphu.WithBaseURLs(base, base), chinhphu.WithLimits(chinhphu.Limits{RequestsPerSecond: 100000, Burst: 10}), chinhphu.WithBackoffs())
	s := New(nil)
	opt, ok := any(s).(chinhphuOption)
	if !ok {
		t.Fatal("chinhphu client option missing")
	}
	opt.WithChinhphu(client)
	return s, stage.Env{Artifacts: artifact.Store{Root: t.TempDir()}}
}

func TestFetch_ChinhphuDownloadFailuresAreNotMissing(t *testing.T) {
	for _, tc := range []struct {
		status int
		data   bool
	}{{200, true}, {500, false}} {
		t.Run(fmt.Sprint(tc.status), func(t *testing.T) {
			token := "abc"
			s, env := secondaryFixture(t, false, &token, tc.status)
			out, err := s.Process(context.Background(), env, stage.Run{Country: "vn"}, stage.Item{Key: "vn:chinhphu:218029"})
			var retryable *stage.RetryableError
			if out.Artifact != nil || (tc.data && !errors.Is(err, stage.ErrSourceData)) || (!tc.data && !errors.As(err, &retryable)) {
				t.Fatalf("output %+v error %v", out, err)
			}
		})
	}
}
func secondaryFetch(t *testing.T, s *Stage, env stage.Env) stage.Output {
	t.Helper()
	out, err := s.Process(context.Background(), env, stage.Run{Country: "vn"}, stage.Item{Key: "vn:chinhphu:218029"})
	if err != nil || out.Artifact == nil || out.State != "done" {
		t.Fatalf("output %+v err %v", out, err)
	}
	return out
}
func TestFetch_ChinhphuBundleHashesFiles(t *testing.T) {
	token := "abc"
	s, env := secondaryFixture(t, false, &token)
	out := secondaryFetch(t, s, env)
	dir := env.Artifacts.Abs(out.Artifact.Path)
	var envelope struct {
		Version int `json:"version"`
		Files   []struct {
			SHA  string `json:"sha256"`
			Path string `json:"path"`
			Size int64  `json:"size"`
		} `json:"files"`
	}
	data, err := os.ReadFile(filepath.Join(dir, "files.json"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte("fetched_at")) {
		t.Fatal("bundle contains acquisition time")
	}
	if err = json.Unmarshal(data, &envelope); err != nil || envelope.Version != 2 || len(envelope.Files) != 1 {
		t.Fatalf("files %s: %v", data, err)
	}
	f := envelope.Files[0]
	pdf, err := os.ReadFile(filepath.Join(dir, f.Path))
	if err != nil || f.SHA != fmt.Sprintf("%x", sha256.Sum256(pdf)) || f.Size != int64(len(pdf)) {
		t.Fatalf("file %+v: %v", f, err)
	}
	if len(out.Observations) != 2 {
		t.Fatalf("observations %+v", out.Observations)
	}
	if sum, verifyErr := artifact.Verify(dir); verifyErr != nil || sum != out.Artifact.SHA256 {
		t.Fatalf("manifest %s: %v", sum, verifyErr)
	}
	for _, observation := range out.Observations {
		body, readErr := os.ReadFile(filepath.Join(dir, observation.Path))
		if readErr != nil || observation.ContentSHA256 != fmt.Sprintf("%x", sha256.Sum256(body)) || observation.Size != int64(len(body)) || observation.FetchedAt == nil || observation.Acquisition != "network" {
			t.Fatalf("observation %+v: %v", observation, readErr)
		}
	}
	for _, name := range []string{"detail.html", "metadata.json", "document.json", "report.json", "MANIFEST.sha256"} {
		if _, err = os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatal(err)
		}
	}
}
func TestFetch_ChinhphuInventoryReconciled(t *testing.T) {
	token := "abc"
	s, env := secondaryFixture(t, true, &token)
	out := secondaryFetch(t, s, env)
	dir := env.Artifacts.Abs(out.Artifact.Path)
	var metadata struct{ Attachments []struct{ Name, URL string } }
	var report struct {
		Missing  []struct{ Name, URL string }
		TooLarge []struct{ Name, URL string }
	}
	for name, dst := range map[string]any{"metadata.json": &metadata, "report.json": &report} {
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		if err = json.Unmarshal(b, dst); err != nil {
			t.Fatal(err)
		}
	}
	if len(metadata.Attachments) != 1 || len(report.Missing) != 1 || metadata.Attachments[0] != report.Missing[0] || len(report.TooLarge) != 0 {
		t.Fatalf("inventory %+v report %+v", metadata, report)
	}
}
func TestFetch_ChinhphuBytesStableAcrossClockAndTokens(t *testing.T) {
	token := "abc"
	s, env := secondaryFixture(t, false, &token)
	env.Now = func() time.Time { return time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC) }
	a := secondaryFetch(t, s, env)
	token = "different-long-token"
	env.Now = func() time.Time { return time.Date(2099, 1, 2, 0, 0, 0, 0, time.UTC) }
	b := secondaryFetch(t, s, env)
	if a.Artifact.SHA256 != b.Artifact.SHA256 {
		t.Fatalf("hashes %s %s", a.Artifact.SHA256, b.Artifact.SHA256)
	}
}

func TestFetch_ChinhphuRejectsContradictoryDetailID(t *testing.T) {
	token := "abc"
	s, env := secondaryFixture(t, false, &token)
	out, err := s.Process(context.Background(), env, stage.Run{Country: "vn"}, stage.Item{Key: "vn:chinhphu:218030"})
	if err == nil && out.Artifact != nil {
		t.Fatal("accepted detail with a conflicting docid")
	}
}

func TestFetch_ChinhphuPreservesRawDates(t *testing.T) {
	token := "abc"
	s, env := secondaryFixture(t, false, &token)
	out := secondaryFetch(t, s, env)
	b, err := os.ReadFile(filepath.Join(env.Artifacts.Abs(out.Artifact.Path), "document.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Document struct {
			Issue     string `json:"raw_issue_date"`
			Effective string `json:"raw_eff_from"`
		} `json:"document"`
	}
	if err = json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Document.Issue != "\n                    30-04-2026" || doc.Document.Effective != "\n                    01-05-2026" {
		t.Fatalf("dates: %q %q", doc.Document.Issue, doc.Document.Effective)
	}
}

func secondaryAliasFixture(t *testing.T) (*Stage, stage.Env) {
	t.Helper()
	raw, err := os.ReadFile("../../chinhphu/testdata/detail-218029.html")
	if err != nil {
		t.Fatal(err)
	}
	var base string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".pdf") {
			w.Header().Set("Content-Type", "application/pdf")
			_, _ = w.Write([]byte("%PDF-test"))
			return
		}
		body := bytes.ReplaceAll(raw, []byte("https://datafiles.chinhphu.vn/cpp/files/"), []byte(base+"/z/"))
		marker := `<a id="ctrl_190596_91_rpTep_ctl00_btn_showfile"`
		alias := `<a href="` + base + `/a/142-2026-ndcp.signed.pdf">alias</a>` + marker
		body = bytes.Replace(body, []byte(marker), []byte(alias), 1)
		_, _ = w.Write(body)
	}))
	base = server.URL
	t.Cleanup(server.Close)
	client := chinhphu.New(chinhphu.WithBaseURLs(base, base), chinhphu.WithLimits(chinhphu.Limits{RequestsPerSecond: 100000, Burst: 10}), chinhphu.WithBackoffs())
	return New(nil).WithChinhphu(client), stage.Env{Artifacts: artifact.Store{Root: t.TempDir()}}
}
func TestFetch_ChinhphuPreservesIdenticalURLAliases(t *testing.T) {
	s, env := secondaryAliasFixture(t)
	out := secondaryFetch(t, s, env)
	_, files, err := readSecondary(env.Artifacts.Abs(out.Artifact.Path))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 || files[0].Name != files[1].Name || files[0].SHA256 != files[1].SHA256 || *files[0].URL >= *files[1].URL || len(out.Observations) != 3 {
		t.Fatalf("files %+v observations %+v", files, out.Observations)
	}
}

func TestFetch_ChinhphuMissingDatePreservesMetadata(t *testing.T) {
	d := chinhphu.Detail{URL: "https://vanban.chinhphu.vn/?docid=1&pageid=27160", Occurrences: []chinhphu.FieldOccurrence{{Name: "Số ký hiệu", Value: "1/2099/QD"}, {Name: "Cơ quan ban hành", Value: "Chính phủ"}}}
	observations := []stage.FetchObservation{}
	err := New(nil).writeChinhphu(context.Background(), t.TempDir(), "1", d, []byte("<html></html>"), &observations)
	if err != nil {
		t.Fatalf("missing date must remain unknown: %v", err)
	}
}

func TestFetch_ChinhphuConflictingIdentityIsGap(t *testing.T) {
	for _, fields := range [][]chinhphu.FieldOccurrence{
		{{Name: "Số ký hiệu", Value: "1/2099/QD"}, {Name: "Số ký hiệu", Value: "2/2099/QD"}},
		{{Name: "Ngày ban hành", Value: "01-01-2099"}, {Name: "Ngày ban hành", Value: "02-01-2099"}},
	} {
		d := chinhphu.Detail{URL: "https://vanban.chinhphu.vn/?docid=1&pageid=27160", Occurrences: fields}
		observations := []stage.FetchObservation{}
		err := New(nil).writeChinhphu(context.Background(), t.TempDir(), "1", d, []byte("<html></html>"), &observations)
		if !errors.Is(err, stage.ErrSourceData) {
			t.Fatalf("conflicting identity: %v", err)
		}
	}
}

func TestFetch_ChinhphuPreservesEndDate(t *testing.T) {
	d := chinhphu.Detail{URL: "https://vanban.chinhphu.vn/?docid=1&pageid=27160", Occurrences: []chinhphu.FieldOccurrence{{Name: "Ngày hết hiệu lực", Value: " 02-01-2099 "}}}
	dir := t.TempDir()
	observations := []stage.FetchObservation{}
	if err := New(nil).writeChinhphu(context.Background(), dir, "1", d, []byte("<html></html>"), &observations); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "document.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc secondaryDocumentEnvelope
	if err = json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Document.EffTo != " 02-01-2099 " {
		t.Fatalf("effective end %q", doc.Document.EffTo)
	}
}

func TestFetch_ChinhphuPreservesUnparsedEffectiveDate(t *testing.T) {
	d := chinhphu.Detail{URL: "https://vanban.chinhphu.vn/?docid=1&pageid=27160", Occurrences: []chinhphu.FieldOccurrence{{Name: "Ngày có hiệu lực", Value: "chưa rõ"}}}
	dir := t.TempDir()
	observations := []stage.FetchObservation{}
	if err := New(nil).writeChinhphu(context.Background(), dir, "1", d, []byte("<html></html>"), &observations); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "document.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc secondaryDocumentEnvelope
	if err = json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Document.EffFrom != "chưa rõ" {
		t.Fatalf("raw date %q", doc.Document.EffFrom)
	}
}

type deadlineTransport struct {
	detail []byte
}

func (t *deadlineTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	body := t.detail
	contentType := "text/html"
	if strings.HasSuffix(req.URL.Path, ".pdf") {
		timer := time.NewTimer(25 * time.Millisecond)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-req.Context().Done():
			return nil, req.Context().Err()
		}
		body = []byte("%PDF-test")
		contentType = "application/pdf"
	}
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{contentType}}, Body: io.NopCloser(bytes.NewReader(body)), Request: req}, nil
}

func TestFetch_ChinhphuItemDeadlineCoversAllAttachments(t *testing.T) {
	const base = "https://test.invalid"
	body := []byte(`<div id="ctrl_190596_91_Content"><table><tr><td>Tài liệu đính kèm</td><td><a href="https://test.invalid/a/first.pdf">first</a><a href="https://test.invalid/b/second.pdf">second</a></td></tr></table></div>`)
	transport := &deadlineTransport{detail: body}
	client := chinhphu.New(chinhphu.WithBaseURLs(base, base), chinhphu.WithTransport(transport), chinhphu.WithLimits(chinhphu.Limits{RequestsPerSecond: 100000, Burst: 10, ItemTimeout: 40 * time.Millisecond}), chinhphu.WithBackoffs())
	out, err := New(nil).WithChinhphu(client).Process(context.Background(), stage.Env{Artifacts: artifact.Store{Root: t.TempDir()}}, stage.Run{Country: "vn"}, stage.Item{Key: "vn:chinhphu:218029"})
	if !errors.Is(err, context.DeadlineExceeded) || out.Artifact != nil {
		t.Fatalf("output=%+v err=%v", out, err)
	}
}
