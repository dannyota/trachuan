//go:build integration

package fixture

import (
	"context"
	"testing"

	"danny.vn/trachuan/internal/testpg"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestCreate_SummaryOption(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		t.Run(map[bool]string{false: "off", true: "on"}[enabled], func(t *testing.T) {
			if enabled {
				t.Setenv("E2E_SUMMARIES", "1")
			} else {
				t.Setenv("E2E_SUMMARIES", "")
			}
			ctx := context.Background()
			name, key, err := Create(ctx)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if e := Drop(ctx, name, key); e != nil {
					t.Error(e)
				}
			})
			admin, base, err := testpg.OpenAdministrator(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = admin.Close(ctx) }()
			base.Path = "/" + name
			db, err := pgxpool.New(ctx, base.String())
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			var docs, provisions, assignments, summary int
			err = db.QueryRow(ctx, `SELECT (SELECT count(*) FROM document),(SELECT count(*) FROM provision),(SELECT count(*) FROM taxonomy_assignment),(SELECT count(*) FROM document WHERE key='vn:fixture:public-summary')`).Scan(&docs, &provisions, &assignments, &summary)
			if err != nil {
				t.Fatal(err)
			}
			if enabled {
				if docs != 29 || provisions != 45 || assignments != 2 || summary != 1 {
					t.Fatalf("counts %d %d %d %d", docs, provisions, assignments, summary)
				}
			} else if docs != 28 || provisions != 43 || assignments != 1 || summary != 0 {
				t.Fatalf("counts %d %d %d %d", docs, provisions, assignments, summary)
			}
			if enabled {
				var valid bool
				err = db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM document d JOIN provision p ON p.document_id=d.id JOIN taxonomy_assignment a ON a.provision_id=p.id JOIN taxonomy_node n ON n.id=a.node_id JOIN taxonomy_node parent ON parent.id=n.parent_id WHERE d.key='vn:fixture:public-summary' AND d.class='primary' AND d.title='Văn bản tóm tắt E2E' AND d.text_source='source_tree' AND p.heading='Điều khoản tóm tắt công khai E2E' AND p.text LIKE '%E2E-BODY-MARKER-7Q4%' AND n.name='Chủ đề tóm tắt E2E' AND n.level=2 AND n.status='active' AND n.provision_count>0 AND parent.level=1 AND parent.status='active' AND a.method='phapdien_seed') AND EXISTS(SELECT 1 FROM provision p JOIN document d ON d.id=p.document_id WHERE d.key='vn:fixture:public-summary' AND char_length(p.heading)=320 AND p.heading LIKE '%E2E-REJECTED-HEADING-9K2%') AND NOT EXISTS(SELECT 1 FROM coverage_gap WHERE document_key='vn:fixture:public-summary' OR item_key='vn:fixture:public-summary')`).Scan(&valid)
				if err != nil || !valid {
					t.Fatalf("summary fixture valid=%t: %v", valid, err)
				}
			}
		})
	}
}
