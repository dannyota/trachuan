//go:build integration

package testpg

import (
	"context"
	"net/url"
	"testing"

	"danny.vn/trachuan/internal/pgdrop"
	"github.com/jackc/pgx/v5"
)

func TestNewDatabase_CreatesAndDropsWithExtensions(t *testing.T) {
	var databaseURL string
	t.Run("create", func(t *testing.T) {
		databaseURL = NewDatabase(t, "vector", "ltree")
		conn, err := pgx.Connect(context.Background(), databaseURL)
		if err != nil {
			t.Fatal(err)
		}
		defer func() {
			if err := conn.Close(context.Background()); err != nil {
				t.Error(err)
			}
		}()
		var count int
		if err := conn.QueryRow(context.Background(), `SELECT count(*) FROM pg_extension WHERE extname IN ('vector','ltree')`).Scan(&count); err != nil || count != 2 {
			t.Fatalf("extensions = %d, error = %v", count, err)
		}
	})
	parsed, err := url.Parse(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	name := parsed.Path[1:]
	parsed.Path = "/postgres"
	conn, err := pgx.Connect(context.Background(), parsed.String())
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := conn.Close(context.Background()); err != nil {
			t.Error(err)
		}
	}()
	var exists bool
	if err := conn.QueryRow(context.Background(), `SELECT EXISTS (SELECT FROM pg_database WHERE datname=$1)`, name).Scan(&exists); err != nil || exists {
		t.Fatalf("database exists = %v, error = %v", exists, err)
	}
}

func TestNewBuilder_NonSuperuserClonesTemplate(t *testing.T) {
	b := NewBuilder(t)
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, b.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := conn.Close(ctx); err != nil {
			t.Error(err)
		}
	}()
	var super, createdb bool
	if err := conn.QueryRow(ctx, `SELECT rolsuper, rolcreatedb FROM pg_roles WHERE rolname=current_user`).Scan(&super, &createdb); err != nil || super || !createdb {
		t.Fatalf("super=%v createdb=%v error=%v", super, createdb, err)
	}
	name := b.Template + "_clone"
	if _, err := conn.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()+" TEMPLATE "+pgx.Identifier{b.Template}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	if err := pgdrop.Drop(ctx, conn, name); err != nil {
		t.Fatal(err)
	}
}
