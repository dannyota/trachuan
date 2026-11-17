//go:build integration

package build

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

type summaryMeasuredTx struct {
	pgx.Tx
	sizes []int
}

type summaryMeasuredRow struct {
	pgx.Row
	sizes *[]int
}

func (r summaryMeasuredRow) Scan(dest ...any) error {
	if err := r.Row.Scan(dest...); err != nil {
		return err
	}
	*r.sizes = append(*r.sizes, *dest[0].(*int))
	return nil
}

func (tx *summaryMeasuredTx) QueryRow(ctx context.Context, query string, args ...any) pgx.Row {
	row := tx.Tx.QueryRow(ctx, query, args...)
	if query == "SELECT octet_length($1::jsonb::text)" {
		return summaryMeasuredRow{Row: row, sizes: &tx.sizes}
	}
	return row
}

func summaryPayloadOctets(t *testing.T, tx pgx.Tx, p summaryPayload) int {
	t.Helper()
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	var size int
	if err = tx.QueryRow(context.Background(), `SELECT octet_length($1::jsonb::text)`, b).Scan(&size); err != nil {
		t.Fatal(err)
	}
	return size
}

func TestLoad_SummaryPayloadSizeBoundary(t *testing.T) {
	f := fixture(t)
	f.fetch(t, "vn:vbpl:1")
	initialCounts, loadErr := f.load(t)
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	if initialCounts.Gaps == 0 {
		t.Fatal("fixture needs committed gap rows")
	}
	ctx := context.Background()
	var id int64
	var raw []byte
	if err := f.corpus.QueryRow(ctx, `SELECT d.id,s.payload FROM document d JOIN document_summary s ON s.document_key=d.key`).Scan(&id, &raw); err != nil {
		t.Fatal(err)
	}
	var baseline summaryPayload
	if err := json.Unmarshal(raw, &baseline); err != nil {
		t.Fatal(err)
	}
	for _, escaped := range []struct{ name, text string }{
		{"plain", "plain"}, {"quotes", `"quoted"`}, {"backslashes", `a\b\\c`},
		{"controls", "\b\f\n\r\t\x01"}, {"vietnamese", "Điều khoản tiếng Việt"}, {"html", strings.Repeat("<>&", 100)},
	} {
		for _, outline := range []string{"headings", "labels", "many_entries", "none"} {
			for _, size := range []int{65535, 65536, 65537} {
				t.Run(fmt.Sprintf("%s/%s/%d", escaped.name, outline, size), func(t *testing.T) {
					tx, err := f.corpus.Begin(ctx)
					if err != nil {
						t.Fatal(err)
					}
					defer func() { _ = tx.Rollback(ctx) }()
					if _, err = tx.Exec(ctx, `DELETE FROM document_summary`); err != nil {
						t.Fatal(err)
					}
					p := baseline
					p.Outline = []summaryOutline{}
					if outline == "many_entries" {
						for i := 1; i <= 1000; i++ {
							p.Outline = append(p.Outline, summaryOutline{Kind: "dieu", Label: "x", Anchor: fmt.Sprint(i)})
						}
						p.HasText = true
						_, err = tx.Exec(ctx, `INSERT INTO provision(id,document_id,node_id,kind,label,heading,ordinal,preorder,path,text,anchor) SELECT i,$1,i::text,'dieu','x','',i,i,('dieu_'||i)::ltree,'Body',i::text FROM generate_series(1,1000) i`, id)
						if err != nil {
							t.Fatal(err)
						}
					} else if outline != "none" {
						heading := ""
						if outline == "headings" {
							heading = "Scope"
						}
						p.Outline = []summaryOutline{{Kind: "dieu", Label: "Điều 1", Anchor: "dieu-1", Heading: heading}}
						p.HasText = true
						_, err = tx.Exec(ctx, `INSERT INTO provision(id,document_id,node_id,kind,label,heading,ordinal,preorder,path,text,anchor) VALUES(1,$1,'1','dieu','Điều 1',$2,1,1,'dieu_1','Body','dieu-1')`, id, heading)
						if err != nil {
							t.Fatal(err)
						}
					}
					p.SourceURL = new("https://example.test/" + escaped.text)
					padding := size - summaryPayloadOctets(t, tx, p)
					if padding < 0 {
						t.Fatal("fixture exceeds target before padding")
					}
					p.SourceURL = new(*p.SourceURL + strings.Repeat("a", padding))
					if measured := summaryPayloadOctets(t, tx, p); measured != size {
						t.Fatalf("fixture size=%d, want %d", measured, size)
					}
					if _, err = tx.Exec(ctx, `UPDATE document SET source_url=$1,text_source='source_tree' WHERE id=$2`, p.SourceURL, id); err != nil {
						t.Fatal(err)
					}
					measured := &summaryMeasuredTx{Tx: tx}
					l := loader{tx: measured, docs: map[string]*loadedDocument{"vn:vbpl:1": {id: id}}, counts: Counts{Gaps: initialCounts.Gaps}}
					err = l.loadSummaries(ctx, nil)
					if size > 65536 && outline == "none" {
						if err != nil {
							t.Fatalf("oversized payload error=%v", err)
						}
						var count int
						if e := tx.QueryRow(ctx, `SELECT count(*) FROM document_summary`).Scan(&count); e != nil || count != 0 {
							t.Fatalf("capped load rows=%d: %v", count, e)
						}
						if len(measured.sizes) != 3 || measured.sizes[2] != size {
							t.Fatalf("capped load measurements=%v", measured.sizes)
						}
						if len(l.gaps) != 2 || l.gaps[0].Kind != "summary_outline_capped" || l.gaps[1].Kind != "summary_row_capped" || l.gaps[1].Detail != "" {
							t.Fatalf("capped row gaps=%+v", l.gaps)
						}
						if err = l.loadGaps(ctx); err != nil {
							t.Fatal(err)
						}
						if err = tx.QueryRow(ctx, `SELECT count(*) FROM coverage_gap WHERE stage='build' AND kind='summary_row_capped' AND document_key='vn:vbpl:1'`).Scan(&count); err != nil || count != 1 {
							t.Fatalf("row cap gaps=%d: %v", count, err)
						}
						return
					}
					if err != nil {
						t.Fatal(err)
					}
					wantChecks, wantOutline := 1, len(p.Outline)
					wantStats := SummaryStats{}
					if size > 65536 {
						if outline == "headings" {
							wantChecks, wantStats.HeadingsDropped = 2, 1
							p.Outline[0].Heading = ""
						} else {
							wantChecks, wantOutline, wantStats.OutlineDropped = 3, 0, 1
							p.Outline = []summaryOutline{}
						}
					}
					if len(measured.sizes) != wantChecks || measured.sizes[0] != size || l.counts.Summary != wantStats || int64(len(l.gaps)) != wantStats.OutlineDropped {
						t.Fatalf("measurements=%v stats=%+v gaps=%d", measured.sizes, l.counts.Summary, len(l.gaps))
					}
					var stored int
					var indexed bool
					if err = tx.QueryRow(ctx, `SELECT octet_length(payload::text),indexed,payload FROM document_summary WHERE document_key='vn:vbpl:1'`).Scan(&stored, &indexed, &raw); err != nil {
						t.Fatal(err)
					}
					if stored != measured.sizes[wantChecks-1] || stored != summaryPayloadOctets(t, tx, p) || stored > 65536 || indexed != (wantOutline != 0) {
						t.Fatalf("stored=%d measured=%v indexed=%t", stored, measured.sizes, indexed)
					}
					var got summaryPayload
					if err = json.Unmarshal(raw, &got); err != nil {
						t.Fatal(err)
					}
					if got.SourceURL == nil || *got.SourceURL != *p.SourceURL || len(got.Outline) != wantOutline {
						t.Fatal("stored payload lost escaped text or outline")
					}
					if !slices.Equal(got.Outline, p.Outline) {
						t.Fatal("stored outline changed")
					}
				})
			}
		}
	}
}
