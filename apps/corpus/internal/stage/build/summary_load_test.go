package build

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type summaryRows struct {
	pgx.Rows
	values [][]any
	index  int
}

func (r *summaryRows) Next() bool { r.index++; return r.index <= len(r.values) }
func (r *summaryRows) Err() error { return nil }
func (r *summaryRows) Close()     {}
func (r *summaryRows) Scan(dest ...any) error {
	values := r.values[r.index-1]
	if len(dest) != len(values) {
		return fmt.Errorf("scan destinations=%d values=%d", len(dest), len(values))
	}
	for i, value := range values {
		target := reflect.ValueOf(dest[i]).Elem()
		if value == nil {
			target.SetZero()
		} else {
			target.Set(reflect.ValueOf(value))
		}
	}
	return nil
}

type summaryTx struct {
	pgx.Tx
	documents []summaryDocument
	inserts   [][]any
	sizes     []int
	sizeCalls int
	sizeErr   error
	errAt     int
}

type summarySizeRow struct {
	size int
	err  error
}

func (r summarySizeRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	*dest[0].(*int) = r.size
	return nil
}

func (tx *summaryTx) QueryRow(_ context.Context, query string, args ...any) pgx.Row {
	tx.sizeCalls++
	if query != "SELECT octet_length($1::jsonb::text)" || len(args) != 1 {
		return summarySizeRow{err: fmt.Errorf("unexpected summary size query")}
	}
	if tx.sizeErr != nil && tx.sizeCalls == tx.errAt {
		return summarySizeRow{err: tx.sizeErr}
	}
	if tx.sizeCalls > len(tx.sizes) {
		return summarySizeRow{err: fmt.Errorf("unexpected summary size measurement")}
	}
	return summarySizeRow{size: tx.sizes[tx.sizeCalls-1]}
}

func (tx *summaryTx) Query(_ context.Context, query string, args ...any) (pgx.Rows, error) {
	rows := &summaryRows{}
	switch {
	case strings.HasPrefix(query, "SELECT key,class"):
		for _, d := range tx.documents {
			p := d.Payload
			rows.values = append(rows.values, []any{d.Key, d.Class, d.StatusClass, d.StatusBasis, d.IsCurrent, d.ValidityConflict, d.EffFrom, d.EffTo, d.TextSource, p.Number, p.Title, p.Type, p.Issuer, p.Issued, p.TextComplete, p.SourceURL, p.HasText})
		}
	case strings.HasPrefix(query, "SELECT kind,label,anchor,heading"):
		for _, n := range tx.documents[args[0].(int64)-1].Payload.Outline {
			rows.values = append(rows.values, []any{n.Kind, n.Label, n.Anchor, n.Heading})
		}
	case strings.HasPrefix(query, "SELECT r.kind"), query == summaryNodesSQL:
	default:
		return nil, fmt.Errorf("unexpected summary query: %s", query)
	}
	return rows, nil
}

func (tx *summaryTx) Exec(_ context.Context, _ string, args ...any) (pgconn.CommandTag, error) {
	tx.inserts = append(tx.inserts, args)
	return pgconn.CommandTag{}, nil
}

func TestLoad_SummaryCapSteps(t *testing.T) {
	whole := summaryFixture()
	headings := summaryOutlineFixture(200, "Điều 1", strings.Repeat("ố", 100))
	outline := summaryOutlineFixture(200, strings.Repeat("ố", 120), "Scope")
	entries := summaryOutlineFixture(1501, "x", "Scope")
	oversized := summaryFixture()
	oversized.Payload.SourceURL = new(strings.Repeat("a", 65536))
	for _, tc := range []struct {
		name      string
		documents []summaryDocument
		headings  int
		outlines  int
		fail      bool
		sizes     []int
	}{
		{"whole", []summaryDocument{whole}, 0, 0, false, []int{65536}},
		{"headings", []summaryDocument{headings}, 1, 0, false, []int{65537, 65536}},
		{"outline_bytes", []summaryDocument{outline}, 0, 1, false, []int{65537, 65537, 65536}},
		{"outline_entries", []summaryDocument{entries}, 0, 1, false, []int{65536, 65536, 65536}},
		{"mixed", []summaryDocument{whole, headings, outline, entries}, 1, 2, false, []int{65536, 65537, 65536, 65537, 65537, 65536, 65536, 65536, 65536}},
		{"still_oversized", []summaryDocument{oversized}, 0, 0, true, []int{65537, 65537, 65537}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tx := &summaryTx{documents: tc.documents, sizes: tc.sizes}
			l := loader{tx: tx, docs: map[string]*loadedDocument{}}
			for i := range tx.documents {
				key := fmt.Sprintf("vn:vbpl:%d", i+1)
				tx.documents[i].Key = key
				l.docs[key] = &loadedDocument{id: int64(i + 1)}
			}
			err := l.loadSummaries(context.Background(), nil)
			if (err != nil) != tc.fail {
				t.Fatalf("load error=%v, want failure=%t", err, tc.fail)
			}
			wantInserts := len(tc.documents)
			if tc.fail {
				wantInserts = 0
			}
			if len(tx.inserts) != wantInserts || len(l.gaps) != tc.outlines {
				t.Fatalf("inserts=%d gaps=%d, want %d and %d", len(tx.inserts), len(l.gaps), wantInserts, tc.outlines)
			}
			for _, gap := range l.gaps {
				if gap.Stage != "build" || gap.Kind != "summary_outline_capped" || gap.Detail != "" {
					t.Errorf("unexpected gap: %+v", gap)
				}
				found := false
				for _, insert := range tx.inserts {
					if insert[0] != gap.ItemKey {
						continue
					}
					found = true
					var p summaryPayload
					if e := json.Unmarshal(insert[11].([]byte), &p); e != nil || insert[3].(bool) || len(p.Outline) != 0 {
						t.Errorf("capped row indexed=%v outline=%d: %v", insert[3], len(p.Outline), e)
					}
				}
				if !found {
					t.Errorf("no row for gap %s", gap.ItemKey)
				}
			}
			stats := l.counts.Summary
			if stats.HeadingsDropped != int64(tc.headings) || stats.OutlineDropped != int64(tc.outlines) {
				t.Errorf("summary stats=%+v, want headings=%d outline=%d", stats, tc.headings, tc.outlines)
			}
		})
	}
}

func TestRecord_SummaryStats(t *testing.T) {
	var record BuildRecord
	if err := json.Unmarshal([]byte(`{"summary":{"headings_dropped":2,"outline_dropped":3}}`), &record); err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err = json.Unmarshal(b, &fields); err != nil {
		t.Fatal(err)
	}
	var stats map[string]int
	if err = json.Unmarshal(fields["summary"], &stats); err != nil {
		t.Fatal(err)
	}
	if len(stats) != 2 || stats["headings_dropped"] != 2 || stats["outline_dropped"] != 3 {
		t.Fatalf("record summary stats=%v", stats)
	}
}

func TestLoad_SummaryUsesDatabaseSize(t *testing.T) {
	for _, tc := range []struct {
		name     string
		sizes    []int
		headings int64
		outlines int64
		fail     bool
	}{
		{"whole", []int{65536}, 0, 0, false},
		{"headings", []int{65537, 65536}, 1, 0, false},
		{"outline", []int{65537, 65537, 65536}, 0, 1, false},
		{"still_over", []int{65537, 65537, 65537}, 0, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tx := &summaryTx{documents: []summaryDocument{summaryFixture()}, sizes: tc.sizes}
			l := loader{tx: tx, docs: map[string]*loadedDocument{"vn:vbpl:1": {id: 1}}}
			err := l.loadSummaries(context.Background(), nil)
			if (err != nil) != tc.fail {
				t.Fatalf("error=%v, want failure=%t", err, tc.fail)
			}
			if l.counts.Summary.HeadingsDropped != tc.headings || l.counts.Summary.OutlineDropped != tc.outlines || int64(len(l.gaps)) != tc.outlines {
				t.Errorf("stats=%+v gaps=%d, want headings=%d outline=%d", l.counts.Summary, len(l.gaps), tc.headings, tc.outlines)
			}
			if tc.fail && len(tx.inserts) != 0 {
				t.Error("oversized payload inserted")
			}
		})
	}
}

func TestLoad_SummarySizingFailure(t *testing.T) {
	for _, at := range []int{1, 2, 3} {
		t.Run(fmt.Sprint(at), func(t *testing.T) {
			cause := errors.New("size query failed")
			tx := &summaryTx{documents: []summaryDocument{summaryFixture()}, sizes: []int{65537, 65537}, sizeErr: cause, errAt: at}
			l := loader{tx: tx, docs: map[string]*loadedDocument{"vn:vbpl:1": {id: 1}}}
			err := l.loadSummaries(context.Background(), nil)
			if !errors.Is(err, cause) || len(tx.inserts) != 0 || len(l.gaps) != 0 || l.counts.Summary != (SummaryStats{}) {
				t.Fatalf("error=%v inserts=%d gaps=%d stats=%+v", err, len(tx.inserts), len(l.gaps), l.counts.Summary)
			}
		})
	}
}
