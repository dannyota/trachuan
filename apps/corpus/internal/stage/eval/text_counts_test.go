package eval

import (
	"context"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/evaluation"
	"github.com/jackc/pgx/v5"
)

type countRow struct {
	class, source, textSource string
	complete                  bool
	n                         int
}
type countRows struct {
	pgx.Rows
	index int
	rows  []countRow
}

func (r *countRows) Next() bool { r.index++; return r.index <= len(r.rows) }
func (*countRows) Close()       {}
func (*countRows) Err() error   { return nil }
func (r *countRows) Scan(dest ...any) error {
	v := r.rows[r.index-1]
	*dest[0].(*string) = v.class
	*dest[1].(*string) = v.source
	*dest[2].(*string) = v.textSource
	*dest[3].(*bool) = v.complete
	*dest[4].(*int) = v.n
	return nil
}

type countDB struct{ sql string }

func (db *countDB) Query(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
	db.sql = sql
	return &countRows{rows: []countRow{{"primary", "vbpl", "source_tree", true, 2}, {"relation_context", "vbpl", "source_tree", false, 1}, {"primary", "chinhphu", "html_transcription", false, 3}}}, nil
}
func TestReport_TextSourceCounts(t *testing.T) {
	db := &countDB{}
	r := evaluation.Report{}
	if err := readDocumentCounts(context.Background(), db, &r); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(db.sql, "text_source") || !strings.Contains(db.sql, "text_complete") {
		t.Fatalf("query %q", db.sql)
	}
	if r.DocumentsByClass["primary"] != 5 || r.DocumentsBySource["vbpl"] != 3 || r.DocumentsByTextSource["source_tree"] != 3 || r.DocumentsByTextSourceAndClass["html_transcription"]["primary"] != 3 || r.DocumentsTextIncomplete != 4 {
		t.Fatalf("counts: %+v", r)
	}
}
