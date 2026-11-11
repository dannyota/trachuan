//go:build integration

package ocrcheck

import (
	"context"
	"errors"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/ocrstore"
	"danny.vn/trachuan/apps/corpus/internal/store"
	"danny.vn/trachuan/apps/corpus/internal/testdb"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

func databaseFixture(t *testing.T) (*Runner, *pgxpool.Pool, *FakeClient, []Document) {
	t.Helper()
	r, _, client, docs := fixture(t)
	db, err := pgxpool.New(context.Background(), testdb.New(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	root := t.TempDir()
	source := ocrstore.Open(root, db)
	if _, err = source.Put(context.Background(), docs[0].Entry.PdfSha256, "vision-ocr/builtin-latest", "banhmi", pgtype.Int4{}, docs[0].Text); err != nil {
		t.Fatal(err)
	}
	entry, err := store.New(db).GetOCRText(context.Background(), store.GetOCRTextParams{PdfSha256: docs[0].Entry.PdfSha256, EngineID: "vision-ocr/builtin-latest"})
	if err != nil {
		t.Fatal(err)
	}
	docs[0].Entry = entry
	r.Repository = NewDatabase(db, root)
	return r, db, client, docs
}

func TestCheck_DatabaseOnlyCompleteResultsArePublished(t *testing.T) {
	for _, tc := range []struct {
		name, stop, text string
		complete         bool
	}{
		{"clean", "end_turn", `{"same_document":true,"pages":10,"items":[]}`, true},
		{"refusal", "refusal", "", false},
		{"partial", "max_tokens", "", false},
		{"schema", "end_turn", `{"same_document":true,"pages":10,"items":null}`, false},
		{"coverage", "end_turn", `{"same_document":true,"pages":1,"items":[]}`, false},
		{"ambiguous verdict", "end_turn", `{"same_document":false,"pages":10,"items":[],"verdict":"clean","Verdict":"rejected"}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, db, c, docs := databaseFixture(t)
			c.Response.Stop = tc.stop
			c.Response.Text = tc.text
			if err := run(t, r, docs, options()); err != nil {
				t.Fatal(err)
			}
			q := store.New(db)
			check, err := q.GetOCRCheck(context.Background(), store.GetOCRCheckParams{OcrTextID: docs[0].Entry.ID, CheckerID: options().CheckerID()})
			if tc.complete {
				if err != nil {
					t.Fatal(err)
				}
				if _, err = ocrstore.DecodeCheck(check.Result); err != nil {
					t.Fatal(err)
				}
			} else if !errors.Is(err, pgx.ErrNoRows) {
				t.Fatalf("unexpected completed check: %v", err)
			}
			var state, reason string
			if err = db.QueryRow(context.Background(), `SELECT state,reason FROM ocr_check_attempt WHERE ocr_text_id=$1`, docs[0].Entry.ID).Scan(&state, &reason); err != nil {
				t.Fatal(err)
			}
			if tc.complete && state != "complete" || !tc.complete && (state != "failed" || reason == "") {
				t.Fatalf("state=%s reason=%s", state, reason)
			}
			text, err := ocrstore.Open(r.Repository.(*Database).root, db).ReadText(docs[0].Entry)
			if err != nil || string(text) != string(docs[0].Text) {
				t.Fatal("stored OCR text changed")
			}
			usage, err := q.SumOCRCheckUsage(context.Background(), options().CheckerID())
			if err != nil || len(usage) != 1 {
				t.Fatal("attempt usage missing")
			}
		})
	}
}
func TestCheck_DatabaseBatchIntentAndResume(t *testing.T) {
	r, db, c, docs := databaseFixture(t)
	o := options()
	o.Batch = true
	if err := run(t, r, docs, o); err != nil {
		t.Fatal(err)
	}
	attempts, err := r.Repository.Open(context.Background(), o.CheckerID())
	if err != nil || len(attempts) != 1 || attempts[0].State != "submitted" || attempts[0].BatchID == "" {
		t.Fatalf("attempts=%+v err=%v", attempts, err)
	}
	c.BatchEnded = true
	c.Results = []BatchResult{{CustomID: customID(attempts[0]), Type: "succeeded", Response: c.Response}}
	if err = run(t, r, docs, o); err != nil {
		t.Fatal(err)
	}
	if c.BatchCalls != 1 {
		t.Fatal("resubmitted paid work")
	}
	if _, err = store.New(db).GetOCRCheck(context.Background(), store.GetOCRCheckParams{OcrTextID: docs[0].Entry.ID, CheckerID: o.CheckerID()}); err != nil {
		t.Fatal(err)
	}
	if open, err := r.Repository.Open(context.Background(), o.CheckerID()); err != nil || len(open) != 0 {
		t.Fatalf("open=%+v err=%v", open, err)
	}
}
