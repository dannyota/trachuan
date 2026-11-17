//go:build integration

package build

import (
	"context"
	"strings"
	"testing"
)

func TestDatabaseMetrics_OnlyRecordedSummaryOmissions(t *testing.T) {
	f := fixture(t)
	f.fetch(t, "vn:vbpl:1")
	f.fetch(t, "vn:vbpl:2")
	loaded, err := f.load(t)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	hash := strings.Repeat("a", 64)
	if _, err = f.corpus.Exec(ctx, `INSERT INTO release VALUES (true,'vn','20990101_1',8,'','{}','{}','{}','{}','{}',$1,$1,$1,'{}',now())`, hash); err != nil {
		t.Fatal(err)
	}
	type gap struct {
		kind, detail, stage, key string
		id                       int64
	}
	for _, tc := range []struct {
		name  string
		keep  bool
		gaps  []gap
		valid bool
	}{
		{"retained", true, []gap{{"summary_field_capped", "number", "build", "vn:vbpl:1", 1}}, true},
		{"number", false, []gap{{"summary_field_capped", "number", "build", "vn:vbpl:1", 1}}, true},
		{"title", false, []gap{{"summary_field_capped", "title", "build", "vn:vbpl:1", 1}}, true},
		{"type", false, []gap{{"summary_field_capped", "type", "build", "vn:vbpl:1", 1}}, true},
		{"issuer", false, []gap{{"summary_field_capped", "issuer", "build", "vn:vbpl:1", 1}}, true},
		{"row", false, []gap{{"summary_row_capped", "", "build", "vn:vbpl:1", 1}}, true},
		{"both", false, []gap{{"summary_outline_capped", "", "build", "vn:vbpl:1", 1}, {"summary_row_capped", "", "build", "vn:vbpl:1", 1}}, true},
		{"two_causes", false, []gap{{"summary_field_capped", "number", "build", "vn:vbpl:1", 1}, {"summary_row_capped", "", "build", "vn:vbpl:1", 1}}, true},
		{"missing_gap", false, nil, false},
		{"outline_only", false, []gap{{"summary_outline_capped", "", "build", "vn:vbpl:1", 1}}, false},
		{"relation_number", false, []gap{{"summary_field_capped", "relation.number", "build", "vn:vbpl:1", 1}}, false},
		{"relation_title", false, []gap{{"summary_field_capped", "relation.title", "build", "vn:vbpl:1", 1}}, false},
		{"topic", false, []gap{{"summary_field_capped", "topic.name", "build", "vn:vbpl:1", 1}}, false},
		{"unknown_field", false, []gap{{"summary_field_capped", "other", "build", "vn:vbpl:1", 1}}, false},
		{"wrong_stage", false, []gap{{"summary_row_capped", "", "extract", "vn:vbpl:1", 1}}, false},
		{"wrong_key", false, []gap{{"summary_row_capped", "", "build", "vn:vbpl:2", 2}}, false},
		{"wrong_id", false, []gap{{"summary_row_capped", "", "build", "vn:vbpl:1", 2}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tx, beginErr := f.corpus.Begin(ctx)
			if beginErr != nil {
				t.Fatal(beginErr)
			}
			defer func() { _ = tx.Rollback(ctx) }()
			if !tc.keep {
				if _, execErr := tx.Exec(ctx, `DELETE FROM document_summary WHERE document_key='vn:vbpl:1'`); execErr != nil {
					t.Fatal(execErr)
				}
			}
			expected := loaded
			for _, g := range tc.gaps {
				expected.Gaps++
				_, execErr := tx.Exec(ctx, `INSERT INTO coverage_gap(id,kind,document_id,document_key,stage,item_key,detail,message,note) VALUES($1,$2,$3,$4,$5,$4,$6,'','')`, expected.Gaps, g.kind, g.id, g.key, g.stage, g.detail)
				if execErr != nil {
					t.Fatal(execErr)
				}
			}
			counts, _, _, metricsErr := databaseMetrics(ctx, f.corpus, expected)
			if !tc.valid {
				if metricsErr == nil || !strings.Contains(metricsErr.Error(), "corpus row counts differ") {
					t.Fatalf("unrecorded omission accepted: %v", metricsErr)
				}
				return
			}
			if metricsErr != nil {
				t.Fatal(metricsErr)
			}
			omitted, ok := counts["document_summary_omitted"]
			wantOmitted := int64(1)
			if tc.keep {
				wantOmitted = 0
			}
			if !ok || omitted != wantOmitted || counts["document_summary"]+omitted != 2 {
				t.Fatalf("summary rows=%d omitted=%d present=%t", counts["document_summary"], omitted, ok)
			}
		})
	}
}
