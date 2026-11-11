package build

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
)

type effectRows struct {
	pgx.Rows
	index  int
	values []effectRow
}
type effectRow struct{ key, heading string }

func (r *effectRows) Next() bool { r.index++; return r.index <= len(r.values) }
func (r *effectRows) Scan(dest ...any) error {
	v := r.values[r.index-1]
	*dest[0].(*string) = v.key
	*dest[1].(*int64) = int64(r.index)
	*dest[2].(**int64) = nil
	*dest[3].(*string) = "dieu"
	*dest[4].(*string) = "Điều 2"
	*dest[5].(*string) = v.heading
	*dest[6].(*string) = "dieu-2"
	*dest[7].(*string) = "Điều 2. Hiệu lực thi hành\nNghị định này có hiệu lực từ ngày 1/2/2021."
	return nil
}
func (*effectRows) Err() error { return nil }
func (*effectRows) Close()     {}

type effectTx struct {
	pgx.Tx
	rows *effectRows
}

func (tx effectTx) Query(context.Context, string, ...any) (pgx.Rows, error) { return tx.rows, nil }

func TestValidity_ParsedHeadingQueuesReview(t *testing.T) {
	docs := map[string]validityDocument{}
	validation := map[string]correctionDocument{}
	for _, tc := range []struct {
		key, source, heading string
		consolidated         bool
	}{
		{"parsed", "html_transcription", "Điều 2. Hiệu lực thi hành", false},
		{"ocr", "ocr", "Điều 2a. Điều khoản thi hành", false},
		{"consolidated", "html_transcription", "Điều 2. Hiệu lực thi hành", true},
		{"tree", "source_tree", "Hiệu lực thi hành", false},
	} {
		d := testDoc(tc.key, "2/2021/NĐ-CP", "2021-01-01", "2021-02-01")
		d.textSource, d.consolidated = tc.source, tc.consolidated
		docs[tc.key] = d
		validation[tc.key] = correctionDocument{provisions: map[string]string{}}
	}
	rows := &effectRows{values: []effectRow{{"parsed", "Điều 2. Hiệu lực thi hành"}, {"ocr", "Điều 2a. Điều khoản thi hành"}, {"consolidated", "Điều 2. Hiệu lực thi hành"}, {"tree", "Hiệu lực thi hành"}}}
	l := loader{tx: effectTx{rows: rows}}
	if err := l.readValidityProvisions(context.Background(), docs, validation); err != nil {
		t.Fatal(err)
	}
	values := make([]validityDocument, 0, len(docs))
	for _, d := range docs {
		values = append(values, d)
	}
	result := runTest(t, values, nil, nil)
	seen := map[string]bool{}
	for _, review := range result.reviews {
		if review.kind == "parsed_text_article" {
			if review.proposed == nil || review.proposed.Evidence.Anchor != "dieu-2" {
				t.Fatalf("missing evidence: %+v", review)
			}
			seen[review.key] = true
		}
	}
	if !seen["parsed"] || !seen["ocr"] || seen["consolidated"] || seen["tree"] {
		t.Fatalf("reviews: %+v", result.reviews)
	}
	if len(result.documents["parsed"].history) != 0 || len(result.documents["ocr"].history) != 0 {
		t.Fatal("parsed source changed validity automatically")
	}
}
