//go:build integration

package retrieval

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestReportLookups_CorpusIdentities(t *testing.T) {
	db, _, f := fixture(t)
	ctx := t.Context()
	if _, err := db.Exec(ctx, `UPDATE document SET number='196-HĐBT',number_norm='196-HDBT' WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `UPDATE document SET number_norm='50/2024/TT-NHNN' WHERE id=2`); err != nil {
		t.Fatal(err)
	}
	e, err := Open(ctx, db, f, fixtureOptions())
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"vn:1", "vn:missing"} {
		doc, found, lookupErr := e.LookupDocument(ctx, key)
		if lookupErr != nil || found != (key == "vn:1") || found && doc.Number != "196-HĐBT" {
			t.Fatalf("%+v %v %v", doc, found, lookupErr)
		}
	}
	for _, tc := range []struct {
		key, anchor string
		found       bool
	}{
		{"vn:1", "p11", true}, {"vn:2", "p21", true}, {"vn:2", "p11", false}, {"vn:missing", "p11", false}, {"vn:1", "missing", false},
	} {
		found, anchorErr := e.AnchorExists(ctx, tc.key, tc.anchor)
		if anchorErr != nil || found != tc.found {
			t.Fatalf("%+v: %v %v", tc, found, anchorErr)
		}
	}
	// Existence accepts duplicate numbers; it must not use read's uniqueness rule.
	if _, err = db.Exec(ctx, `UPDATE document SET number_norm='196-HDBT' WHERE id=3`); err != nil {
		t.Fatal(err)
	}
	for _, number := range []string{"196-HDBT", "50/2024/TT-NHNN", "missing"} {
		found, numberErr := e.HasNumber(ctx, number)
		if numberErr != nil || found != (number != "missing") {
			t.Fatalf("%s: %v %v", number, found, numberErr)
		}
	}
	want := map[string]struct{}{"196": {}, "HDBT": {}, "50": {}, "2024": {}, "TT": {}, "NHNN": {}, "n": {}}
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for segment := range want {
				found, segmentErr := e.HasNumberSegment(ctx, segment)
				if segmentErr != nil || !found {
					t.Errorf("segment=%q found=%v err=%v", segment, found, segmentErr)
				}
			}
		})
	}
	wg.Wait()
	if len(e.numberSegments) != len(want) {
		t.Fatal("unexpected number segments")
	}
	if _, err = db.Exec(ctx, `UPDATE document SET number_norm='PRIVATE'`); err != nil {
		t.Fatal(err)
	}
	for segment := range want {
		if found, segmentErr := e.HasNumberSegment(ctx, segment); segmentErr != nil || !found {
			t.Fatal("number segments not pinned to engine", segmentErr)
		}
	}
}

func TestReportSegments_CorpusBudget(t *testing.T) {
	db, _, embedder := fixture(t)
	ctx := t.Context()
	_, err := db.Exec(ctx, `UPDATE document SET number_norm=CASE id WHEN 1 THEN repeat('A',32) WHEN 2 THEN repeat('B',33) END WHERE id IN (1,2)`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(ctx, `INSERT INTO document (id,country,key,number,number_norm,title,doc_type,issuer,class,source,validity_conflict,raw_issue_date,raw_eff_from,raw_eff_to,status_code,status_name,status_class,status_basis,is_current,is_consolidated,content_state,fetch_sha256,text_source,text_complete)
SELECT 1000+i,d.country,'vn:segment:'||i,n.value,n.value,d.title,d.doc_type,d.issuer,d.class,d.source,d.validity_conflict,d.raw_issue_date,d.raw_eff_from,d.raw_eff_to,d.status_code,d.status_name,d.status_class,d.status_basis,d.is_current,d.is_consolidated,d.content_state,d.fetch_sha256,d.text_source,d.text_complete
FROM document d CROSS JOIN generate_series(0,12499) AS g(i)
CROSS JOIN LATERAL (SELECT format('S%s/S%s/S%s/S%s',lpad((i*4)::text,5,'0'),lpad((i*4+1)::text,5,'0'),lpad((i*4+2)::text,5,'0'),lpad((i*4+3)::text,5,'0')) AS value) n
WHERE d.id=3`)
	if err != nil {
		t.Fatal(err)
	}
	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
	defer slog.SetDefault(previous)
	engine, err := Open(ctx, db, embedder, fixtureOptions())
	if err != nil {
		t.Fatal(err)
	}
	if len(engine.numberSegments) != 50_000 {
		t.Fatalf("count=%d", len(engine.numberSegments))
	}
	decoder := json.NewDecoder(&logs)
	var event map[string]any
	if err = decoder.Decode(&event); err != nil || event["msg"] != "report number segment limit reached" || event["release"] != "test" {
		t.Fatalf("segment limit event=%v err=%v", event, err)
	}
	if err = decoder.Decode(&event); !errors.Is(err, io.EOF) {
		t.Fatalf("unexpected additional segment event: %v", err)
	}
	for _, segment := range []string{strings.Repeat("A", 32), "S49998"} {
		if ok, segmentErr := engine.HasNumberSegment(ctx, segment); !ok || segmentErr != nil {
			t.Fatalf("missing allowed segment %q", segment)
		}
	}
	for _, segment := range []string{strings.Repeat("B", 33), "S49999"} {
		if ok, segmentErr := engine.HasNumberSegment(ctx, segment); ok || segmentErr != nil {
			t.Fatalf("accepted over-budget segment %q", segment)
		}
	}
}

func TestReportLookups_CanceledAndDatabaseFailure(t *testing.T) {
	db, e, _ := fixture(t)
	_, _, lookupErr := e.LookupDocument(t.Context(), "vn:\x00PRIVATE")
	if lookupErr == nil || strings.Contains(lookupErr.Error(), "PRIVATE") || !errors.Is(lookupErr, ErrUnavailable) {
		t.Fatal("unsafe document lookup failure", lookupErr)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, _, err := e.LookupDocument(ctx, "vn:1"); err == nil {
		t.Fatal("canceled document lookup")
	}
	if _, err := e.AnchorExists(ctx, "vn:1", "p11"); err == nil {
		t.Fatal("canceled anchor lookup")
	}
	if _, err := e.HasNumber(ctx, "n"); err == nil {
		t.Fatal("canceled number lookup")
	}
	if _, err := db.Exec(t.Context(), `DROP TABLE document CASCADE`); err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.LookupDocument(t.Context(), "vn:1"); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
}

type reportLookupTrace struct {
	t       *testing.T
	queries int
}

func (trace *reportLookupTrace) TraceQueryStart(ctx context.Context, conn *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	if strings.HasPrefix(data.SQL, "SELECT number FROM document") || strings.HasPrefix(data.SQL, "SELECT EXISTS") || strings.HasPrefix(data.SQL, "SELECT DISTINCT segment") {
		trace.queries++
		var timeout, readonly string
		if err := conn.QueryRow(ctx, `SELECT current_setting('statement_timeout'),current_setting('transaction_read_only')`).Scan(&timeout, &readonly); err != nil {
			trace.t.Error(err)
		} else if timeout != "2s" || readonly != "on" {
			trace.t.Errorf("deadline=%s readonly=%s", timeout, readonly)
		}
	}
	return ctx
}

func (*reportLookupTrace) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func TestReportLookups_ServingDeadlineAndOneSegmentQuery(t *testing.T) {
	db, _, f := fixture(t)
	cfg, err := pgxpool.ParseConfig(db.Config().ConnString())
	if err != nil {
		t.Fatal(err)
	}
	trace := &reportLookupTrace{t: t}
	cfg.ConnConfig.Tracer = trace
	traced, err := pgxpool.NewWithConfig(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(traced.Close)
	options := fixtureOptions()
	options.Profile = "serving"
	e, err := Open(t.Context(), traced, f, options)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = e.LookupDocument(t.Context(), "vn:1"); err != nil {
		t.Fatal(err)
	}
	if _, err = e.AnchorExists(t.Context(), "vn:1", "p11"); err != nil {
		t.Fatal(err)
	}
	if _, err = e.HasNumber(t.Context(), "n"); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if _, err = e.HasNumberSegment(t.Context(), "n"); err != nil {
			t.Fatal(err)
		}
	}
	if trace.queries != 4 {
		t.Fatalf("queries=%d, want one segment query and three identity lookups", trace.queries)
	}
}
