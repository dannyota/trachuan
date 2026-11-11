package retrieval

import (
	"context"
	"errors"
	"net"
	"testing"

	"danny.vn/trachuan/internal/embedding"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/puddle/v2"
)

func TestValidateDenseRecipe_RejectsMismatch(t *testing.T) {
	query := embedding.QueryRecipe1()
	doc := embedding.DocumentRecipe1()
	dense := DenseRecipe{Version: 1, Model: query.Model, Dimensions: query.Dimensions, Document: doc, DocumentHash: doc.Hash(), Query: query, QueryHash: query.Hash()}
	if err := validateDenseRecipe(dense, query, query.Dimensions); err != nil {
		t.Fatal(err)
	}
	dense.QueryHash = doc.Hash()
	if err := validateDenseRecipe(dense, query, query.Dimensions); err == nil {
		t.Fatal("accepted a mismatched query hash")
	}
	dense.QueryHash = query.Hash()
	if err := validateDenseRecipe(dense, query, query.Dimensions-1); err == nil {
		t.Fatal("accepted a mismatched typmod")
	}
}

func TestReadRelease_RejectsMissingDatabase(t *testing.T) {
	if _, err := ReadRelease(context.Background(), nil); err == nil {
		t.Fatal("accepted missing database")
	}
}

func TestReadRelease_PreservesDatabaseError(t *testing.T) {
	ctx := context.Background()
	cfg, err := pgxpool.ParseConfig("postgres://reader@localhost/test?sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.DialFunc = func(context.Context, string, string) (net.Conn, error) {
		t.Error("unexpected network access")
		return nil, errors.New("network disabled")
	}
	db, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	if _, err := ReadRelease(ctx, db); !errors.Is(err, puddle.ErrClosedPool) {
		t.Fatalf("read error=%v", err)
	}
	if _, err := Open(ctx, db, nil, Options{}); !errors.Is(err, puddle.ErrClosedPool) {
		t.Fatalf("open error=%v", err)
	}
}
