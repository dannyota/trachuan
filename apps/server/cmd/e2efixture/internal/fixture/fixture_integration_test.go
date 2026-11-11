//go:build integration

package fixture

import (
	"context"
	"regexp"
	"testing"

	"danny.vn/trachuan/internal/testpg"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestCreateAndDrop_Fixture(t *testing.T) {
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
	if name == "" || key == "" {
		t.Fatal("missing fixture identity")
	}
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
	rows, err := db.Query(ctx, `SELECT key FROM document`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	grammar := regexp.MustCompile(`^vn:[a-z]+:[A-Za-z0-9._-]+$`)
	count := 0
	for rows.Next() {
		var documentKey string
		if err := rows.Scan(&documentKey); err != nil {
			t.Fatal(err)
		}
		if !grammar.MatchString(documentKey) {
			t.Errorf("invalid document key %q", documentKey)
		}
		count++
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if count == 0 {
		t.Fatal("fixture has no documents")
	}
}
