//go:build integration

package testpg

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestDropCorpusDatabase_RequiresOwnMarker(t *testing.T) {
	ctx := context.Background()
	admin, _ := administrator(t)
	name := "corpus_vn_" + Release(t)
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { DropCorpusDatabase(t, admin, name, "vn/"+Release(t)) })
	foreign := "20990101"
	if Release(t) == foreign {
		foreign = "20990102"
	}
	if _, err := admin.Exec(ctx, "COMMENT ON DATABASE "+pgx.Identifier{name}.Sanitize()+" IS '{\"run_key\":\"vn/"+foreign+"\"}'"); err != nil {
		t.Fatal(err)
	}
	DropCorpusDatabase(t, admin, name, "vn/"+Release(t))
	var exists bool
	if err := admin.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname=$1)`, name).Scan(&exists); err != nil || !exists {
		t.Fatalf("foreign database missing: %v", err)
	}
	if _, err := admin.Exec(ctx, "COMMENT ON DATABASE "+pgx.Identifier{name}.Sanitize()+" IS '{\"run_key\":\"vn/"+Release(t)+"\"}'"); err != nil {
		t.Fatal(err)
	}
	DropCorpusDatabase(t, admin, name, "vn/"+Release(t))
	if err := admin.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname=$1)`, name).Scan(&exists); err != nil || exists {
		t.Fatalf("own database remains: %v", err)
	}
}
