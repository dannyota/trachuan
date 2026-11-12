//go:build integration

package fixture

import (
	"context"
	"strings"
	"testing"

	"danny.vn/trachuan/internal/testpg"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestCreateAndDrop_Taxonomy(t *testing.T) {
	ctx := context.Background()
	name, key, err := Create(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if cleanupErr := Drop(ctx, name, key); cleanupErr != nil {
			t.Error(cleanupErr)
		}
	})
	admin, base, err := testpg.OpenAdministrator(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Close(ctx) })
	base.Path = "/" + name
	db, err := pgxpool.New(ctx, base.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	rows, err := db.Query(ctx, `SELECT code, provision_count, document_count FROM taxonomy_node ORDER BY level`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var codes []string
	for rows.Next() {
		var code string
		var provisions, documents int
		if err := rows.Scan(&code, &provisions, &documents); err != nil {
			t.Fatal(err)
		}
		if provisions != 1 || documents != 1 {
			t.Errorf("%s counts: %d provisions, %d documents", code, provisions, documents)
		}
		codes = append(codes, code)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if strings.Join(codes, ",") != "90,90.1,90.1.c1" {
		t.Fatalf("nodes: %v", codes)
	}
	var assignments, matching int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM taxonomy_assignment a JOIN taxonomy_node n ON n.id=a.node_id JOIN taxonomy_node f ON f.id=a.family_id WHERE n.code='90.1.c1' AND f.code='90.1' AND a.method='phapdien_seed' AND NOT a.exclude AND a.confidence=1 AND a.taxonomy_version='v1'`).Scan(&assignments); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(ctx, `SELECT count(*) FROM chunk WHERE content LIKE '%phân loại giả lập%'`).Scan(&matching); err != nil {
		t.Fatal(err)
	}
	if assignments != 1 || matching != 24 {
		t.Fatalf("assignments=%d matching chunks=%d", assignments, matching)
	}
}
