//go:build integration

package retrieval

import (
	"context"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

type sparseTrace struct{ rescoring, scans atomic.Int64 }

func (s *sparseTrace) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	if data.SQL == sparseRescoreSQL {
		s.rescoring.Add(1)
	}
	if strings.HasPrefix(data.SQL, "SELECT c.id, -(c.sparse") {
		s.scans.Add(1)
	}
	return ctx
}
func (*sparseTrace) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func startFixturePostings(t *testing.T, e *Engine) {
	t.Helper()
	e.options.Profile = "serving"
	t.Cleanup(e.Close)
	outcome := make(chan PostingsOutcome, 1)
	e.StartSparsePostings(func(result PostingsOutcome) { outcome <- result })
	result := <-outcome
	if result.Status != "ready" || !e.SparsePostingsReady() || result.Bytes <= 0 || result.EstimatedPeakBytes < result.Bytes {
		t.Fatalf("postings outcome: %+v", result)
	}
	e.options.Profile = "evaluation"
}

func TestSparsePostings_CollectionsEqualSQL(t *testing.T) {
	trace := &sparseTrace{}
	db, e, _ := fixtureWithTrace(t, trace)
	ctx := context.Background()
	for _, statement := range []string{
		`UPDATE document SET title='ngân hàng',eff_from=NULL,eff_to='2099-01-02' WHERE id=1`,
		`UPDATE document SET eff_from='2099-01-02',eff_to=NULL,status_class='not_yet',is_current=false WHERE id=2`,
		`UPDATE document SET class='relation_context',number_norm='50/2024/TT-NHNN',validity_conflict=true WHERE id=3`,
		`UPDATE chunk SET sparse=(SELECT sparse FROM chunk WHERE id=1) WHERE id>1`,
	} {
		if _, err := db.Exec(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	startFixturePostings(t, e)
	before := time.Date(2099, 1, 1, 16, 59, 59, 0, time.UTC)
	cases := []struct {
		name      string
		req       Request
		notYet    bool
		emptyRefs bool
	}{
		{name: "default", req: Request{Query: "ngân hàng"}},
		{name: "cursor before midnight", req: Request{Query: "ngân hàng", Instant: before}},
		{name: "cursor after midnight", req: Request{Query: "ngân hàng", Instant: before.Add(time.Second)}},
		{name: "as of null start", req: Request{Query: "ngân hàng", AsOf: &Date{2099, 1, 1}}},
		{name: "as of boundary", req: Request{Query: "ngân hàng", AsOf: &Date{2099, 1, 2}}},
		{name: "context present conflict", req: Request{Query: "ngân hàng 50/2024/TT-NHNN"}},
		{name: "context absent", req: Request{Query: "ngân hàng 51/2024/TT-NHNN"}},
		{name: "context empty", req: Request{Query: "ngân hàng 50/2024/TT-NHNN"}, emptyRefs: true},
		{name: "historical context conflict", req: Request{Query: "ngân hàng 50/2024/TT-NHNN", AsOf: &Date{2099, 1, 2}}},
		{name: "not yet", req: Request{Query: "ngân hàng"}, notYet: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			prepared, err := e.Prepare(ctx, tc.req)
			if err != nil {
				t.Fatal(err)
			}
			if tc.emptyRefs {
				prepared.Validation.References = []string{}
			}
			for _, mode := range []Mode{"exact", "ann"} {
				for _, depth := range []int{1, 2, 10} {
					prepared.Request.Mode = mode
					depths := Depths{Dense: 4, Lexical: depth}
					index := e.sparse.index.Swap(nil)
					sqlBefore := e.LexicalSources().SQL
					collect := e.Collect
					if tc.notYet {
						collect = e.CollectWithNotYetEligible
					}
					want, referenceErr := collect(ctx, prepared, depths)
					e.sparse.index.Store(index)
					if referenceErr != nil {
						t.Fatal(referenceErr)
					}
					if e.LexicalSources().SQL != sqlBefore+1 {
						t.Fatal("reference did not serve SQL")
					}
					count := e.LexicalSources().Postings
					rescoring, scans := trace.rescoring.Load(), trace.scans.Load()
					got, collectErr := collect(ctx, prepared, depths)
					if collectErr != nil {
						t.Fatal(collectErr)
					}
					if !e.SparsePostingsReady() || e.LexicalSources().Postings != count+1 || trace.rescoring.Load() != rescoring+1 || trace.scans.Load() != scans {
						t.Fatal("collection did not use one postings rescore")
					}
					if !reflect.DeepEqual(got, want) {
						t.Fatalf("%s depth %d collection differs from SQL", mode, depth)
					}
				}
			}
		})
	}
	e.Retire()
	if !e.SparsePostingsReady() {
		t.Fatal("retirement discarded leased postings")
	}
	count := e.LexicalSources().Postings
	collectFixture(t, e, Request{Query: "ngân hàng"}, Depths{4, 4})
	if e.LexicalSources().Postings != count+1 {
		t.Fatal("retired lease lost postings")
	}
}

func TestSparsePostings_InvalidStoredWeightKeepsSQL(t *testing.T) {
	db, e, _ := fixture(t)
	if _, err := db.Exec(t.Context(), `UPDATE chunk SET sparse='{1:-1}/1048576'::sparsevec WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	e.options.Profile = "serving"
	t.Cleanup(e.Close)
	outcome := make(chan PostingsOutcome, 1)
	e.StartSparsePostings(func(result PostingsOutcome) { outcome <- result })
	if result := <-outcome; result.Status != "disabled" || result.Reason == "" || e.SparsePostingsReady() {
		t.Fatalf("invalid snapshot: %+v", result)
	}
	before := e.LexicalSources().SQL
	collectFixture(t, e, Request{Query: "ngân hàng"}, Depths{4, 4})
	if e.LexicalSources().SQL != before+1 {
		t.Fatal("disabled postings did not serve SQL")
	}
}

func TestSparsePostings_CloseAndRetireDuringLoad(t *testing.T) {
	for _, retire := range []bool{false, true} {
		t.Run(map[bool]string{false: "closed", true: "retired"}[retire], func(t *testing.T) {
			db, e, _ := fixture(t)
			e.options.Profile = "serving"
			t.Cleanup(e.Close)
			conn, err := db.Acquire(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			outcome := make(chan PostingsOutcome, 1)
			e.StartSparsePostings(func(result PostingsOutcome) { outcome <- result })
			if retire {
				e.Retire()
			} else {
				e.Close()
			}
			conn.Release()
			if result := <-outcome; result.Status != "disabled" || e.SparsePostingsReady() {
				t.Fatalf("late publication: %+v", result)
			}
		})
	}
}

func TestSparsePostings_CloseWaitsForRunningLoader(t *testing.T) {
	_, e, _ := fixture(t)
	e.options.Profile = "serving"
	entered, release, finished := make(chan struct{}), make(chan struct{}), make(chan struct{})
	e.StartSparsePostings(func(PostingsOutcome) { close(entered); <-release })
	<-entered
	go func() { e.Close(); close(finished) }()
	<-e.sparse.ctx.Done()
	select {
	case <-finished:
		t.Fatal("Close returned before loader exited")
	default:
	}
	close(release)
	<-finished
	if e.SparsePostingsReady() {
		t.Fatal("Close retained postings")
	}
}
