//go:build integration

package cli

import (
	"context"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/ocrstore"
	"danny.vn/trachuan/apps/corpus/internal/ocrvision"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestCheck_AllEntriesAndReview(t *testing.T) {
	ctx := context.Background()
	env, hashes := visionRunFixture(t)
	source := ocrstore.Open(env.root, env.db)
	checker := "claude-opus-5-5/high/ocr-check-3"
	engines := []string{"vision-ocr/builtin-latest", ocrvision.EngineID}
	for _, engine := range engines {
		if _, err := source.Put(ctx, hashes[0], engine, "vision", pgtype.Int4{Int32: 1, Valid: true}, []byte("text")); err != nil {
			t.Fatal(err)
		}
		entries, err := source.Lookup(ctx, []string{hashes[0]}, []string{engine}, checker)
		if err != nil {
			t.Fatal(err)
		}
		result := []byte(`{"same_document":true,"pages":1,"pdf_pages":1,"items":[],"verdict":"clean"}`)
		if _, err = source.PutCheck(ctx, entries[hashes[0]].Text, checker, result, 0, true); err != nil {
			t.Fatal(err)
		}
	}
	one, err := env.checkDocuments(ctx, checker, false)
	if err != nil || len(one) != 1 || one[0].Entry.EngineID != engines[0] {
		t.Fatalf("preferred=%+v err=%v", one, err)
	}
	all, err := env.checkDocuments(ctx, checker, true)
	if err != nil || len(all) != 2 || all[0].Entry.ID == all[1].Entry.ID {
		t.Fatalf("all=%+v err=%v", all, err)
	}
	review, err := env.review(ctx, checker, true)
	if err != nil || len(review) != 2 {
		t.Fatalf("review=%+v err=%v", review, err)
	}
}
