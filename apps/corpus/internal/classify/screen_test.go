package classify

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/batchapi"
	"github.com/anthropics/anthropic-sdk-go/v2"
)

type memoryLedger struct{ rows []Submission }

func (m *memoryLedger) Lock(context.Context) (func(), error) { return func() {}, nil }
func (m *memoryLedger) List(context.Context) ([]Submission, error) {
	return append([]Submission(nil), m.rows...), nil
}
func (m *memoryLedger) Save(_ context.Context, s Submission) error {
	for i, v := range m.rows {
		if v.ID == s.ID {
			m.rows[i] = s
			return nil
		}
	}
	m.rows = append(m.rows, s)
	return nil
}

func (m *memoryLedger) Restore(ctx context.Context, submissions []Submission) error {
	rows := append([]Submission(nil), m.rows...)
	for _, s := range submissions {
		if err := m.Save(ctx, s); err != nil {
			m.rows = rows
			return err
		}
	}
	return nil
}

type fakeClient struct {
	batchCreated                     time.Time
	counts, creates, gets, downloads int
	input                            int64
	createErr                        error
	ended                            bool
	results                          []batchapi.Result
	beforeCreate                     func([]batchapi.Request)
}

func (f *fakeClient) CountTokens(context.Context, anthropic.MessageNewParams) (int64, error) {
	f.counts++
	return f.input, nil
}
func (f *fakeClient) Create(_ context.Context, r []batchapi.Request) (anthropic.MessageBatch, error) {
	f.creates++
	if f.beforeCreate != nil {
		f.beforeCreate(r)
	}
	return anthropic.MessageBatch{ID: "batch_synthetic", CreatedAt: f.batchCreated}, f.createErr
}
func (f *fakeClient) Get(context.Context, string) (anthropic.MessageBatch, error) {
	f.gets++
	b := anthropic.MessageBatch{ID: "batch_synthetic"}
	if f.ended {
		b.ProcessingStatus = "ended"
	}
	return b, nil
}
func (f *fakeClient) Results(context.Context, string) ([]batchapi.Result, error) {
	f.downloads++
	return f.results, nil
}

func screenFixture(t *testing.T) (*Runner, *memoryLedger, *fakeClient, Options) {
	t.Helper()
	m := &memoryLedger{}
	f := &fakeClient{input: 100, ended: true}
	now := time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC)
	r := &Runner{Repository: m, Client: f, IntentDir: filepath.Join(t.TempDir(), "intents"), Now: func() time.Time { return now }, Wait: func(context.Context, time.Duration) error { return errors.New("interrupted") }}
	recipe, _ := NewRecipe("claude-haiku-5-5", strings.Repeat("a", 64))
	o := Options{RunID: 1, Country: "vn", Release: "20990101", Documents: 1, Seed: 7, Recipe: recipe, Budget: "pilot", BudgetMicroUSD: 15000000, Out: filepath.Join(t.TempDir(), "screen")}
	f.beforeCreate = func(req []batchapi.Request) {
		if len(m.rows) != 1 || m.rows[0].State != "preparing" {
			t.Fatal("provider called before row")
		}
		s := m.rows[0]
		if _, err := os.Stat(filepath.Join(r.IntentDir, "submissions", s.ID+".json")); err != nil {
			t.Fatal("provider called before durable intent")
		}
		if req[0].CustomID != s.ID+"-000000" {
			t.Fatal("wrong custom id")
		}
		var msg anthropic.Message
		if err := msg.UnmarshalJSON([]byte(`{"stop_reason":"end_turn","content":[{"type":"text","text":"{\"codes\":[\"1.1\",\"retired\"]}"}],"usage":{"input_tokens":100,"output_tokens":10}}`)); err != nil {
			t.Fatal(err)
		}
		f.results = []batchapi.Result{{CustomID: req[0].CustomID, Type: "succeeded", Message: msg}}
	}
	return r, m, f, o
}
func documents() []Document {
	return []Document{{Key: "vn:vbpl:1", Title: "Synthetic legal title", Fields: []string{"Synthetic field"}}}
}
func families() []Family {
	return []Family{{Code: "1.1", Name: "Synthetic family", ParentName: "Synthetic topic"}}
}

func TestScreen_DryRunNeverWritesOrSubmits(t *testing.T) {
	r, m, f, o := screenFixture(t)
	o.DryRun = true
	s, err := r.Screen(context.Background(), o, documents(), families())
	if err != nil || s.Requests != 1 || s.InputTokens != 100 || s.ReserveMicroUSD != 1029 || !s.Admitted || f.creates != 0 || len(m.rows) != 0 {
		t.Fatalf("summary %+v rows %d creates %d error %v", s, len(m.rows), f.creates, err)
	}
	if _, err = os.Stat(r.IntentDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("dry run wrote intent directory")
	}
}

func TestScreen_ResultsSettleAndCompletionIsNoOp(t *testing.T) {
	r, m, f, o := screenFixture(t)
	_, err := r.Screen(context.Background(), o, documents(), families())
	if err != nil {
		t.Fatal(err)
	}
	if len(m.rows) != 1 || m.rows[0].State != "completed" || !m.rows[0].Settled || m.rows[0].CostMicroUSD != 8 {
		t.Fatalf("ledger %+v", m.rows)
	}
	b, err := os.ReadFile(filepath.Join(o.Out, "families.json"))
	if err != nil || !strings.Contains(string(b), `"retired"`) {
		t.Fatalf("artifact %s error %v", b, err)
	}
	before := f.counts + f.creates + f.gets + f.downloads
	_, err = r.Screen(context.Background(), o, documents(), families())
	if err != nil {
		t.Fatal(err)
	}
	if before != f.counts+f.creates+f.gets+f.downloads {
		t.Fatal("completed screen called provider")
	}
}

func TestScreen_UncertainSubmissionBlocksSpending(t *testing.T) {
	r, m, f, o := screenFixture(t)
	f.createErr = errors.New("synthetic connection lost")
	if _, err := r.Screen(context.Background(), o, documents(), families()); err == nil {
		t.Fatal("accepted lost create response")
	}
	if m.rows[0].State != "uncertain" || m.rows[0].Settled {
		t.Fatal("lost reservation")
	}
	f.createErr = nil
	if _, err := r.Screen(context.Background(), o, documents(), families()); err == nil || f.creates != 1 {
		t.Fatal("resubmitted uncertain batch")
	}
}

func TestScreen_AdmissionCountsEveryUnsettledRow(t *testing.T) {
	for _, state := range []string{"submitted", "failed", "abandoned", "cancelled"} {
		t.Run(state, func(t *testing.T) {
			r, m, f, o := screenFixture(t)
			m.rows = []Submission{{ID: strings.Repeat("b", 32), RunID: 2, State: state, ReserveMicroUSD: 14999000}}
			if _, err := r.Screen(context.Background(), o, documents(), families()); err == nil || f.creates != 0 {
				t.Fatal("overspent reserved budget")
			}
		})
	}
}

func TestScreen_ResumesAndEnforcesDeadline(t *testing.T) {
	r, m, f, o := screenFixture(t)
	f.ended = false
	if _, err := r.Screen(context.Background(), o, documents(), families()); err == nil {
		t.Fatal("ignored interruption")
	}
	if m.rows[0].State != "submitted" {
		t.Fatal("lost submitted batch")
	}
	r.Now = func() time.Time { return time.Date(2099, 1, 3, 0, 0, 0, 0, time.UTC) }
	if _, err := r.Screen(context.Background(), o, documents(), families()); err == nil {
		t.Fatal("ignored deadline")
	}
	if f.creates != 1 || m.rows[0].State != "failed" || m.rows[0].Settled {
		t.Fatal("deadline released reservation or resubmitted")
	}
}

func TestScreen_CompletedOutputSurvivesUnrelatedUncertainty(t *testing.T) {
	r, m, _, o := screenFixture(t)
	if _, err := r.Screen(context.Background(), o, documents(), families()); err != nil {
		t.Fatal(err)
	}
	m.rows = append(m.rows, Submission{ID: strings.Repeat("b", 32), RunID: 2, State: "uncertain"})
	if _, err := r.Screen(context.Background(), o, documents(), families()); err != nil {
		t.Fatal("completed screen was blocked", err)
	}
}

func TestScreen_RefusesOccupiedOutputBeforeProviderCalls(t *testing.T) {
	r, m, f, o := screenFixture(t)
	if err := os.MkdirAll(o.Out, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(o.Out, "unrelated"), []byte("synthetic"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Screen(context.Background(), o, documents(), families()); err == nil || f.creates != 0 || f.counts != 0 || len(m.rows) != 0 {
		t.Fatal("occupied output reached provider")
	}
}

func TestCancel_SettlesPartialResultsBeforeCancelled(t *testing.T) {
	r, m, f, o := screenFixture(t)
	f.ended = false
	if _, err := r.Screen(context.Background(), o, documents(), families()); err == nil {
		t.Fatal("expected interrupted polling")
	}
	f.ended = true
	if err := r.Cancel(context.Background(), m.rows[0].ID, o.Out); err != nil {
		t.Fatal(err)
	}
	if m.rows[0].State != "cancelled" || !m.rows[0].Settled || m.rows[0].CostMicroUSD != 8 {
		t.Fatalf("row %+v", m.rows[0])
	}
}

func TestScreen_DeadlineUsesProviderCreationTime(t *testing.T) {
	r, m, f, o := screenFixture(t)
	f.ended = false
	f.batchCreated = r.Now().Add(time.Minute)
	if _, err := r.Screen(context.Background(), o, documents(), families()); err == nil {
		t.Fatal("expected interrupted polling")
	}
	if !m.rows[0].DeadlineAt.Equal(f.batchCreated.Add(26 * time.Hour)) {
		t.Fatal("deadline does not follow provider submission time")
	}
}
