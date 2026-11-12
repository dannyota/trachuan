//go:build integration

package ocrcheck

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

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

func TestCheck_DatabaseIntentVisibleBeforeProviderCall(t *testing.T) {
	for _, batch := range []bool{false, true} {
		t.Run(fmt.Sprint(batch), func(t *testing.T) {
			r, db, c, docs := databaseFixture(t)
			o := options()
			o.Batch = batch
			expected := int64(320200)
			if batch {
				expected = 160100
			}
			before := func() {
				observer := NewDatabase(db, r.Repository.(*Database).root)
				open, err := observer.Open(context.Background(), o.CheckerID())
				if err != nil || len(open) != 1 || open[0].State != "preparing" || open[0].Usage.CountedInput != 100 || open[0].Usage.MaxOutput != 32000 {
					t.Fatalf("provider preceded committed intent: %+v %v", open, err)
				}
				spent, err := observer.Spent(context.Background(), o.CheckerID())
				if err != nil || spent != expected {
					t.Fatalf("provider preceded durable reservation: %d %v", spent, err)
				}
			}
			r.Client = &intentClient{FakeClient: c, before: before}
			if err := run(t, r, docs, o); err != nil {
				t.Fatal(err)
			}
		})
	}
}

type intentClient struct {
	*FakeClient
	before func()
}

func (c *intentClient) Direct(ctx context.Context, r Request) (Response, error) {
	c.before()
	return c.FakeClient.Direct(ctx, r)
}
func (c *intentClient) CreateBatch(ctx context.Context, r []Request) (Batch, error) {
	c.before()
	return c.FakeClient.CreateBatch(ctx, r)
}

func TestCheck_DatabaseUncertainAndAbandonedChargesSurvive(t *testing.T) {
	r, db, c, docs := databaseFixture(t)
	r.Client = &directClient{FakeClient: c, call: func(context.Context, Request) (Response, error) { return Response{}, context.DeadlineExceeded }}
	if err := run(t, r, docs, options()); err == nil {
		t.Fatal("unknown final charge was released")
	}
	observer := NewDatabase(db, r.Repository.(*Database).root)
	open, err := observer.Open(context.Background(), options().CheckerID())
	if err != nil || len(open) != 1 || open[0].State != "uncertain" {
		t.Fatalf("unknown billing was not durable: %+v %v", open, err)
	}
	r.Repository = observer
	spent, err := observer.Spent(context.Background(), options().CheckerID())
	if err != nil || spent != 320200 {
		t.Fatalf("lost reservation after reopening: %d %v", spent, err)
	}
	r.Now = func() time.Time { return open[0].CreatedAt.Add(24 * time.Hour) }
	o := options()
	o.Abandon = open[0].SubmissionID
	if err = run(t, r, nil, o); err != nil {
		t.Fatal(err)
	}
	spent, err = NewDatabase(db, observer.root).Spent(context.Background(), o.CheckerID())
	if err != nil || spent != 320200 {
		t.Fatalf("abandonment released charge: %d %v", spent, err)
	}
	if _, err = store.New(db).GetOCRCheck(context.Background(), store.GetOCRCheckParams{OcrTextID: docs[0].Entry.ID, CheckerID: o.CheckerID()}); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("uncertain check was published")
	}
}

func TestCheck_DatabaseVerifiableEvidence(t *testing.T) {
	for _, tc := range []struct {
		name, quote, pdf, reason string
		dropped                  int
	}{
		{"unverifiable", "invented reading", "different", "ocr_quote_not_found", 0},
		{"equal", "public OCR text", "public\nOCR text", "", 1},
		{"different", "public OCR text", "different", "", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, db, c, docs := databaseFixture(t)
			body, _ := json.Marshal(map[string]any{"same_document": true, "pages": 10, "items": []map[string]any{{"kind": "amount", "page": 1, "ocr": tc.quote, "pdf": tc.pdf, "note": "difference"}}})
			c.Response.Text = string(body)
			if err := run(t, r, docs, options()); err != nil {
				t.Fatal(err)
			}
			var reason string
			if err := db.QueryRow(context.Background(), `SELECT reason FROM ocr_check_attempt WHERE ocr_text_id=$1`, docs[0].Entry.ID).Scan(&reason); err != nil {
				t.Fatal(err)
			}
			if reason != tc.reason {
				t.Fatalf("reason=%s", reason)
			}
			check, err := store.New(db).GetOCRCheck(context.Background(), store.GetOCRCheckParams{OcrTextID: docs[0].Entry.ID, CheckerID: options().CheckerID()})
			if tc.reason != "" {
				if !errors.Is(err, pgx.ErrNoRows) {
					t.Fatal("failed quote published")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			result, err := ocrstore.DecodeCheck(check.Result)
			if err != nil || result.DroppedEqual != tc.dropped || len(result.Items) != 1-tc.dropped {
				t.Fatalf("result=%+v error=%v", result, err)
			}
		})
	}
}

func TestCheck_DatabaseKeepsCheckerVersionsSeparate(t *testing.T) {
	r, db, client, docs := databaseFixture(t)
	source := ocrstore.Open(r.Repository.(*Database).root, db)
	entry := docs[0].Entry
	oldChecker := "claude-sonnet-5-5/medium/ocr-check-1"
	newChecker := "claude-sonnet-5-5/medium/ocr-check-2"
	legacy := []byte(`{"same_document":false,"pages":10,"pdf_pages":10,"items":[],"verdict":"rejected"}`)
	if _, err := source.PutCheck(context.Background(), entry, oldChecker, legacy, 0, true); err != nil {
		t.Fatal(err)
	}
	before, err := source.Lookup(context.Background(), []string{entry.PdfSha256}, []string{entry.EngineID}, newChecker)
	if err != nil || before[entry.PdfSha256].Check != nil {
		t.Fatalf("v1 satisfied v2 lookup: %v", err)
	}
	if err = run(t, r, docs, options()); err != nil {
		t.Fatal(err)
	}
	if client.DirectCalls != 1 {
		t.Fatal("v1 check suppressed v2 request")
	}
	ids := map[int64]bool{}
	for _, tc := range []struct{ checker, verdict string }{{oldChecker, "rejected"}, {newChecker, "clean"}} {
		entries, lookupErr := source.Lookup(context.Background(), []string{entry.PdfSha256}, []string{entry.EngineID}, tc.checker)
		if lookupErr != nil {
			t.Fatal(lookupErr)
		}
		check := entries[entry.PdfSha256].Check
		if check == nil || check.CheckerID != tc.checker || ids[check.ID] {
			t.Fatal("checker versions shared a row")
		}
		ids[check.ID] = true
		result, decodeErr := ocrstore.DecodeCheck(check.Result)
		if decodeErr != nil || result.Verdict != tc.verdict {
			t.Fatalf("checker=%s verdict=%s error=%v", tc.checker, result.Verdict, decodeErr)
		}
	}
}

func TestCheck_DatabaseInsertRejection(t *testing.T) {
	for _, tc := range []struct {
		code     string
		rejected bool
	}{{"22P05", true}, {"22021", true}, {"23514", false}} {
		t.Run(tc.code, func(t *testing.T) {
			r, db, _, docs := databaseFixture(t)
			ctx := context.Background()
			_, err := db.Exec(ctx, `CREATE FUNCTION reject_check() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'forced insert failure' USING ERRCODE = '`+tc.code+`'; END $$`)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = db.Exec(ctx, `CREATE TRIGGER reject_check BEFORE INSERT ON ocr_check FOR EACH ROW EXECUTE FUNCTION reject_check()`); err != nil {
				t.Fatal(err)
			}
			summary, err := r.Run(ctx, docs, options())
			if !tc.rejected {
				if err == nil {
					t.Fatal("unexpected success")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			var state, reason string
			var data []byte
			if err = db.QueryRow(ctx, `SELECT state, reason, usage FROM ocr_check_attempt WHERE ocr_text_id=$1`, docs[0].Entry.ID).Scan(&state, &reason, &data); err != nil {
				t.Fatal(err)
			}
			var usage Usage
			if err = json.Unmarshal(data, &usage); err != nil {
				t.Fatal(err)
			}
			if state != "failed" || reason != "store_rejected" || !usage.Final || usage.Input != 100 || usage.Output != 100 || usage.CostMicroUSD != 1200 || summary.Failed != 1 || summary.Complete != 0 {
				t.Fatalf("state=%s reason=%s usage=%+v summary=%+v", state, reason, usage, summary)
			}
			if _, err = store.New(db).GetOCRCheck(ctx, store.GetOCRCheckParams{OcrTextID: docs[0].Entry.ID, CheckerID: options().CheckerID()}); !errors.Is(err, pgx.ErrNoRows) {
				t.Fatalf("check error=%v", err)
			}
		})
	}
}
