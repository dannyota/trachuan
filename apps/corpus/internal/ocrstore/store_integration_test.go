//go:build integration

package ocrstore

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/testdb"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestStore_EntriesImmutable(t *testing.T) {
	ctx := context.Background()
	db, err := pgxpool.New(ctx, testdb.New(t))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	dir := t.TempDir()
	s := Open(dir, db)
	pdf := fmt.Sprintf("%x", sha256.Sum256([]byte("pdf")))
	data := []byte("OCR text")
	present, err := s.Put(ctx, pdf, "engine-a", "banhmi", pgtype.Int4{}, data)
	if err != nil || present {
		t.Fatalf("first put: present=%v err=%v", present, err)
	}
	entries, err := s.Lookup(ctx, []string{pdf}, []string{"missing", "engine-a"}, "checker")
	if err != nil || entries[pdf].Check != nil || entries[pdf].Text.EngineID != "engine-a" {
		t.Fatalf("lookup: %+v %v", entries, err)
	}
	entry := entries[pdf].Text
	absent, err := s.Lookup(ctx, []string{pdf}, []string{"missing"}, "checker")
	if err != nil || len(absent) != 0 {
		t.Fatalf("missing engine: %+v %v", absent, err)
	}
	path := filepath.Join(dir, "text", entry.TextSha256+".txt")
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	present, err = s.Put(ctx, pdf, "engine-a", "banhmi", pgtype.Int4{}, data)
	if err != nil || !present {
		t.Fatalf("second put: present=%v err=%v", present, err)
	}
	present, err = s.Put(ctx, pdf, "engine-a", "vision", pgtype.Int4{Int32: 1, Valid: true}, []byte("different text"))
	if err != nil || !present {
		t.Fatalf("conflicting put: present=%v err=%v", present, err)
	}
	if present, err = s.Put(ctx, pdf, "engine-b", "vision", pgtype.Int4{Int32: 1, Valid: true}, []byte("other engine")); err != nil || present {
		t.Fatalf("second engine: present=%v err=%v", present, err)
	}
	entries, err = s.Lookup(ctx, []string{pdf}, []string{"engine-b", "engine-a"}, "")
	if err != nil || entries[pdf].Text.EngineID != "engine-b" || entries[pdf].Check != nil {
		t.Fatalf("engine order: %+v %v", entries, err)
	}
	entries, err = s.Lookup(ctx, []string{pdf}, []string{"engine-a"}, "")
	if err != nil || entries[pdf].Text.TextSha256 != entry.TextSha256 || entries[pdf].Text.Origin != "banhmi" || entries[pdf].Text.Pages.Valid {
		t.Fatalf("entry changed: %+v %v", entries, err)
	}
	after, err := os.Stat(path)
	if err != nil || !os.SameFile(before, after) || !before.ModTime().Equal(after.ModTime()) {
		t.Fatalf("text rewritten: %v", err)
	}
	if b, readErr := s.ReadText(entry); readErr != nil || string(b) != string(data) {
		t.Fatalf("read text: %v", readErr)
	}
	for _, query := range []string{"UPDATE ocr_text SET origin='vision' WHERE id=$1", "DELETE FROM ocr_text WHERE id=$1"} {
		if _, execErr := db.Exec(ctx, query, entry.ID); execErr == nil {
			t.Fatalf("mutation succeeded: %s", query)
		}
	}
	if _, checkErr := s.PutCheck(ctx, entry.ID, "checker", json.RawMessage(`{"z":2,"a":1}`), 2, false); checkErr == nil {
		t.Fatal("incomplete check accepted")
	}
	present, err = s.PutCheck(ctx, entry.ID, "checker", json.RawMessage(`{"z":2,"a":1}`), 2, true)
	if err != nil || present {
		t.Fatalf("put check: %v %v", present, err)
	}
	present, err = s.PutCheck(ctx, entry.ID, "checker", json.RawMessage(`{"a":9}`), 0, true)
	if err != nil || !present {
		t.Fatalf("duplicate check: %v %v", present, err)
	}
	entries, err = s.Lookup(ctx, []string{pdf}, []string{"engine-a"}, "checker")
	if err != nil || entries[pdf].Check == nil {
		t.Fatalf("checked lookup: %+v %v", entries, err)
	}
	check := entries[pdf].Check
	want := fmt.Sprintf("%x", sha256.Sum256([]byte(`{"a":1,"z":2}`)))
	if check.ResultSha256 != want || check.Flagged != 2 {
		t.Fatalf("unexpected check: %+v", check)
	}
	for _, query := range []string{"UPDATE ocr_check SET flagged=0 WHERE id=$1", "DELETE FROM ocr_check WHERE id=$1"} {
		if _, err := db.Exec(ctx, query, check.ID); err == nil {
			t.Fatalf("mutation succeeded: %s", query)
		}
	}
}
