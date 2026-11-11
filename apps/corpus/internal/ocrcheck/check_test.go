package ocrcheck

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/ocrstore"
	"danny.vn/trachuan/apps/corpus/internal/store"
)

func fixture(t *testing.T) (*Runner, *memoryRepository, *FakeClient, []Document) {
	t.Helper()
	pdf := []byte("test PDF")
	path := filepath.Join(t.TempDir(), "pdf")
	if err := os.WriteFile(path, pdf, 0600); err != nil {
		t.Fatal(err)
	}
	entry := store.OcrText{ID: 1, PdfSha256: fmt.Sprintf("%x", sha256.Sum256(pdf))}
	repo := &memoryRepository{checks: map[int64]ocrstore.CheckResult{}}
	client := &FakeClient{Response: Response{Stop: "end_turn", Text: `{"same_document":true,"pages":10,"items":[]}`, Usage: Usage{Input: 100, Output: 100}}, Tokens: 100}
	runner := &Runner{Client: client, Repository: repo, CountPages: func(context.Context, []byte) (int, error) { return 10, nil }, Now: func() time.Time { return time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC) }}
	return runner, repo, client, []Document{{Entry: entry, PDFPath: path, Text: []byte("public OCR text")}}
}
func options() Options { return Options{Model: "claude-sonnet-5-5", Effort: "medium", MaxUSD: 10} }
func run(t *testing.T, r *Runner, docs []Document, o Options) error {
	t.Helper()
	_, err := r.Run(context.Background(), docs, o)
	return err
}

func TestCheck_ParsesDisagreements(t *testing.T) {
	r, s, c, d := fixture(t)
	c.Response.Text = `{"same_document":true,"pages":10,"items":[{"kind":"amount","page":2,"ocr":"1","pdf":"2","note":"different amount"}]}`
	if err := run(t, r, d, options()); err != nil {
		t.Fatal(err)
	}
	got := s.checks[1]
	if got.Verdict != "flagged" || len(got.Items) != 1 || got.Items[0].PDF != "2" {
		t.Fatalf("result=%+v", got)
	}
}
func TestCheck_VerdictRules(t *testing.T) {
	for _, tc := range []struct{ body, want string }{
		{`{"same_document":true,"pages":10,"items":[]}`, "clean"},
		{`{"same_document":false,"pages":10,"items":[]}`, "rejected"},
		{`{"same_document":true,"pages":10,"items":[{"kind":"missing_text","page":1,"ocr":"","pdf":"x","note":"x"}]}`, "flagged"},
		{`{"same_document":true,"pages":10,"items":[{"kind":"missing_text","page":1,"ocr":"","pdf":"x","note":"x"},{"kind":"missing_text","page":2,"ocr":"","pdf":"y","note":"y"}]}`, "rejected"},
	} {
		t.Run(tc.want+tc.body, func(t *testing.T) {
			r, s, c, d := fixture(t)
			c.Response.Text = tc.body
			if err := run(t, r, d, options()); err != nil {
				t.Fatal(err)
			}
			if s.checks[1].Verdict != tc.want {
				t.Fatal(s.checks)
			}
		})
	}
}
func TestCheck_PartialOrMalformedIsIncomplete(t *testing.T) {
	for _, tc := range []struct{ stop, body, reason string }{
		{"refusal", "", "refusal"}, {"max_tokens", "", "max_tokens"}, {"model_context_window_exceeded", "", "context_window"},
		{"end_turn", "{", "schema"}, {"end_turn", `{"same_document":true,"pages":10,"items":[],"extra":1}`, "schema"},
		{"end_turn", `{"same_document":true,"pages":0,"items":[]}`, "schema"},
	} {
		t.Run(tc.reason+tc.body, func(t *testing.T) {
			r, s, c, d := fixture(t)
			c.Response.Stop = tc.stop
			c.Response.Text = tc.body
			if err := run(t, r, d, options()); err != nil {
				t.Fatal(err)
			}
			if len(s.checks) != 0 || s.attempts[0].Reason != tc.reason || s.attempts[0].State != "failed" {
				t.Fatalf("%+v %v", s.attempts, s.checks)
			}
			if s.cost == 0 {
				t.Fatal("failed usage not billed")
			}
		})
	}
}
func TestCheck_NeverChangesStoredText(t *testing.T) {
	r, _, _, d := fixture(t)
	before := string(d[0].Text)
	if err := run(t, r, d, options()); err != nil {
		t.Fatal(err)
	}
	if string(d[0].Text) != before {
		t.Fatal("text changed")
	}
}
func TestCheck_MaxUSDStopsBeforeSpending(t *testing.T) {
	r, s, c, d := fixture(t)
	o := options()
	o.MaxUSD = .01
	if err := run(t, r, d, o); !errors.Is(err, ErrBudget) {
		t.Fatalf("error=%v", err)
	}
	if c.DirectCalls != 0 || len(s.attempts) != 0 {
		t.Fatal("spent before budget check")
	}
}
func TestCheck_OverLimitPDF(t *testing.T) {
	r, s, c, d := fixture(t)
	f, err := os.OpenFile(d[0].PDFPath, os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.Truncate(30_000_001); err != nil {
		t.Fatal(err)
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
	if err = run(t, r, d, options()); err != nil {
		t.Fatal(err)
	}
	if c.DirectCalls != 0 || s.attempts[0].Reason != "over_check_limit" {
		t.Fatal(s.attempts)
	}
}
func TestCheck_PDFHashVerified(t *testing.T) {
	r, s, c, d := fixture(t)
	d[0].Entry.PdfSha256 = strings.Repeat("0", 64)
	if err := run(t, r, d, options()); err != nil {
		t.Fatal(err)
	}
	if c.DirectCalls != 0 || s.attempts[0].Reason != "pdf_hash_mismatch" {
		t.Fatal(s.attempts)
	}
}
func TestCheck_PageCountMustMatch(t *testing.T) {
	for _, unknown := range []bool{false, true} {
		t.Run(fmt.Sprint(unknown), func(t *testing.T) {
			r, s, c, d := fixture(t)
			reason := "page_count_mismatch"
			if unknown {
				reason = "page_count_unknown"
				r.CountPages = func(context.Context, []byte) (int, error) { panic("bad PDF") }
			} else {
				c.Response.Text = `{"same_document":true,"pages":1,"items":[]}`
			}
			if err := run(t, r, d, options()); err != nil {
				t.Fatal(err)
			}
			if len(s.checks) != 0 || s.attempts[0].Reason != reason {
				t.Fatal(s.attempts)
			}
		})
	}
}
func TestCheck_StrictResultDecoding(t *testing.T) {
	for _, body := range []string{
		`{"same_document":false,"pages":1,"items":[],"verdict":"clean","Verdict":"rejected"}`,
		`{"same_document":true,"same_document":false,"pages":10,"items":[]}`,
		`{"Same_document":true,"pages":10,"items":[]}`,
		`{"same_document":true,"pages":10,"items":[{"kind":"date","page":1,"ocr":"x","OCR":"y","pdf":"z","note":"x"}]}`,
		`{"same_document":true,"pages":10,"items":null}`,
	} {
		t.Run(body, func(t *testing.T) {
			r, s, c, d := fixture(t)
			c.Response.Text = body
			if err := run(t, r, d, options()); err != nil {
				t.Fatal(err)
			}
			if len(s.checks) != 0 || s.attempts[0].Reason != "schema" {
				t.Fatal(s.attempts)
			}
		})
	}
}
func TestCheck_ResumeCollectsSubmittedBatch(t *testing.T) {
	r, s, c, d := fixture(t)
	o := options()
	o.Batch = true
	if err := run(t, r, d, o); err != nil {
		t.Fatal(err)
	}
	if s.attempts[0].State != "submitted" {
		t.Fatal(s.attempts)
	}
	c.BatchEnded = true
	c.Results = []BatchResult{{CustomID: s.attempts[0].SubmissionID + "-1", Type: "succeeded", Response: c.Response}}
	if err := run(t, r, d, o); err != nil {
		t.Fatal(err)
	}
	if c.BatchCalls != 1 || s.checks[1].Verdict != "clean" {
		t.Fatal(s.attempts)
	}
}
func TestCheck_BatchSplitsBySize(t *testing.T) {
	r, _, c, d := fixture(t)
	second := d[0]
	second.Entry.ID = 2
	d = append(d, second)
	o := options()
	o.Batch = true
	r.batchBytes = 1_000
	r.requestBytes = func(Request) int { return 600 }
	if err := run(t, r, d, o); err != nil {
		t.Fatal(err)
	}
	if c.BatchCalls != 2 {
		t.Fatalf("batches=%d", c.BatchCalls)
	}
}
func TestCheck_UncertainSubmissionBlocksResubmit(t *testing.T) {
	r, s, c, d := fixture(t)
	o := options()
	o.Batch = true
	c.CreateError = errors.New("transport failure")
	if err := run(t, r, d, o); err == nil {
		t.Fatal("expected uncertain error")
	}
	c.CreateError = nil
	if err := run(t, r, d, o); err == nil {
		t.Fatal("uncertain did not block")
	}
	if c.BatchCalls != 1 || s.attempts[0].State != "uncertain" {
		t.Fatal(s.attempts)
	}
}
func TestCheck_ReconcileAttachesOrAbandons(t *testing.T) {
	for _, found := range []bool{false, true} {
		t.Run(fmt.Sprint(found), func(t *testing.T) {
			r, s, c, d := fixture(t)
			o := options()
			o.Batch = true
			c.CreateError = errors.New("lost response")
			_ = run(t, r, d, o)
			c.CreateError = nil
			r.Now = func() time.Time { return s.attempts[0].CreatedAt.Add(25 * time.Hour) }
			if found {
				c.Batches = []Batch{{ID: "found", Ended: true, CreatedAt: s.attempts[0].CreatedAt}}
				c.Results = []BatchResult{{CustomID: s.attempts[0].SubmissionID + "-1", Type: "succeeded", Response: c.Response}}
			}
			o.Reconcile = true
			if err := run(t, r, d, o); err != nil {
				t.Fatal(err)
			}
			want := "abandoned"
			if found {
				want = "submitted"
			}
			if s.attempts[0].State != want {
				t.Fatal(s.attempts)
			}
		})
	}
}
func TestClients_NoCredentialInLogsOrArtifacts(t *testing.T) {
	r, s, c, d := fixture(t)
	secret := "fake-key-never-log"
	text := "fake-document-never-log"
	d[0].Text = []byte(text)
	c.DirectError = errors.New(secret + text)
	err := run(t, r, d, options())
	data, _ := json.Marshal(s.attempts)
	if strings.Contains(fmt.Sprint(err)+string(data), secret) || strings.Contains(fmt.Sprint(err)+string(data), text) {
		t.Fatal("sensitive data escaped")
	}
}

type memoryRepository struct {
	attempts []Attempt
	checks   map[int64]ocrstore.CheckResult
	cost     int64
}

func (s *memoryRepository) Open(context.Context, string) ([]Attempt, error) {
	var a []Attempt
	for _, v := range s.attempts {
		if v.State == "preparing" || v.State == "submitted" || v.State == "uncertain" {
			a = append(a, v)
		}
	}
	return a, nil
}
func (s *memoryRepository) Spent(context.Context, string) (int64, error) { return s.cost, nil }
func (s *memoryRepository) Completed(_ context.Context, id int64, _ string) (bool, error) {
	_, ok := s.checks[id]
	return ok, nil
}
func (s *memoryRepository) Begin(_ context.Context, entries []store.OcrText, checker, mode, submission string, now time.Time) ([]Attempt, error) {
	var a []Attempt
	for _, e := range entries {
		v := Attempt{ID: int64(len(s.attempts) + 1), EntryID: e.ID, Checker: checker, Mode: mode, SubmissionID: submission, State: "preparing", CreatedAt: now}
		s.attempts = append(s.attempts, v)
		a = append(a, v)
	}
	return a, nil
}
func (s *memoryRepository) Submitted(_ context.Context, submission, batch string) error {
	for i := range s.attempts {
		if s.attempts[i].SubmissionID == submission {
			s.attempts[i].State = "submitted"
			s.attempts[i].BatchID = batch
		}
	}
	return nil
}
func (s *memoryRepository) Uncertain(_ context.Context, submission string) error {
	changed := false
	for i := range s.attempts {
		if s.attempts[i].SubmissionID == submission && s.attempts[i].State == "preparing" {
			s.attempts[i].State = "uncertain"
			changed = true
		}
	}
	if !changed {
		return fmt.Errorf("no preparing attempts")
	}
	return nil
}
func (s *memoryRepository) Abandon(_ context.Context, submission string) error {
	for i := range s.attempts {
		if s.attempts[i].SubmissionID == submission {
			s.attempts[i].State = "abandoned"
		}
	}
	return nil
}
func (s *memoryRepository) Finish(_ context.Context, a Attempt, _ store.OcrText, u Usage, result *ocrstore.CheckResult, reason string) error {
	for i := range s.attempts {
		if s.attempts[i].ID == a.ID {
			s.attempts[i].State = "failed"
			s.attempts[i].Reason = reason
			if result != nil {
				s.attempts[i].State = "complete"
				s.checks[a.EntryID] = *result
			}
		}
	}
	s.cost += u.CostMicroUSD
	return nil
}
