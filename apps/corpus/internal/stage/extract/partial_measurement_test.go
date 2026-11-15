//go:build measurement

package extract

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/stage/fetch"
	"danny.vn/trachuan/apps/corpus/internal/store"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPartialTreeMeasurement_Inventory(t *testing.T) {
	ctx := context.Background()
	cc, rel, out := os.Getenv("MEASURE_COUNTRY"), os.Getenv("MEASURE_REL"), os.Getenv("MEASURE_OUT")
	dbURL, root := os.Getenv("CORPUS_DATABASE_URL"), os.Getenv("CORPUS_ARTIFACTS")
	if cc == "" || rel == "" || out == "" || dbURL == "" || root == "" {
		t.Fatal("set MEASURE_COUNTRY, MEASURE_REL, MEASURE_OUT, CORPUS_DATABASE_URL and CORPUS_ARTIFACTS")
	}
	db, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	q := store.New(db)
	run, err := q.GetRun(ctx, store.GetRunParams{Country: cc, Rel: rel})
	if err != nil {
		t.Fatal(err)
	}
	extracts, err := q.ListStageItemArtifacts(ctx, store.ListStageItemArtifactsParams{RunID: run.ID, Stage: "extract"})
	if err != nil {
		t.Fatal(err)
	}
	file, err := os.Create(out)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	w := csv.NewWriter(file)
	w.Comma = '\t'
	if err = w.Write([]string{"document_key", "tree_chars", "transcription_chars", "decision", "lost_anchors"}); err != nil {
		t.Fatal(err)
	}
	artifacts := artifact.Store{Root: root}
	for _, item := range extracts {
		if item.State != "done" || item.Path == nil || item.Sha256 == nil {
			continue
		}
		dir := artifacts.Abs(*item.Path)
		sum, verifyErr := artifact.Verify(dir)
		if verifyErr != nil || sum != *item.Sha256 {
			t.Fatalf("verify extract %s: %v", item.ItemKey, verifyErr)
		}
		meta, readErr := os.ReadFile(filepath.Join(dir, "text.json"))
		if readErr == nil {
			var record textRecord
			if err = json.Unmarshal(meta, &record); err != nil {
				t.Fatal(err)
			}
			if record.TextSource != "source_tree" {
				continue
			}
		} else if !errors.Is(readErr, os.ErrNotExist) {
			t.Fatal(readErr)
		}
		data, readErr := os.ReadFile(filepath.Join(dir, "provisions.json"))
		if readErr != nil {
			t.Fatal(readErr)
		}
		var rows []provision
		if err = json.Unmarshal(data, &rows); err != nil {
			t.Fatal(err)
		}
		input, readErr := q.GetExtractInput(ctx, store.GetExtractInputParams{RunID: run.ID, ItemKey: item.ItemKey})
		if readErr != nil {
			t.Fatal(readErr)
		}
		bundle := artifacts.Abs(input.Path)
		sum, verifyErr = artifact.Verify(bundle)
		if verifyErr != nil || sum != input.Sha256 {
			t.Fatalf("verify fetch %s: %v", item.ItemKey, verifyErr)
		}
		doc, readErr := fetch.ReadDocument(bundle, item.ItemKey)
		if readErr != nil {
			t.Fatal(readErr)
		}
		parts := strings.SplitN(item.ItemKey, ":", 3)
		if len(parts) != 3 {
			t.Fatal("invalid document key")
		}
		transcription, readErr := selectFiles(bundle, parts[2], doc.Files, absentOCR{})
		if readErr != nil {
			t.Fatal(readErr)
		}
		treeChars, textChars, _, _ := partialTreeLengths(rows, transcription)
		selected, _, readErr := choose(ctx, bundle, item.ItemKey)
		if readErr != nil {
			t.Fatal(readErr)
		}
		decision, lost := "keep", ""
		if selected.source == "html_transcription" {
			decision = "switch"
			lost = lostTreeAnchors(rows, selected)
		} else if selected.gap == "tree_text_partial" || selected.extraGap == "tree_text_partial" {
			decision = "tree_text_partial"
		}
		if err = w.Write([]string{item.ItemKey, strconv.Itoa(treeChars), strconv.Itoa(textChars), decision, lost}); err != nil {
			t.Fatal(err)
		}
	}
	w.Flush()
	if err = w.Error(); err != nil {
		t.Fatal(fmt.Errorf("write measurement: %w", err))
	}
}

func lostTreeAnchors(rows []provision, selected selection) string {
	_, _, parsed := parseTranscription(selected.data, selected.name, selected.hash)
	anchors := make(map[string]bool)
	for _, p := range parsed.Provisions {
		anchors[p.Anchor] = true
	}
	var lost []string
	for _, row := range rows {
		if !anchors[row.Path] {
			lost = append(lost, row.Path)
		}
	}
	return strings.Join(lost, ",")
}

func TestLostTreeAnchors_StructureOnly(t *testing.T) {
	rows := []provision{
		{Kind: "phan", Path: "phan-i"},
		{Kind: "chuong", Path: "chuong-i"},
		{Kind: "muc", Path: "muc-1"},
		{Kind: "other", Path: "other-1"},
		{Kind: "dieu", Path: "dieu-1"},
	}
	selected := selection{data: []byte("<p>Điều 1. Phạm vi</p><p>" + strings.Repeat("Nội dung văn bản. ", 30) + "</p>")}
	if got := lostTreeAnchors(rows, selected); got != "phan-i,chuong-i,muc-1,other-1" {
		t.Fatalf("lost anchors = %q", got)
	}
}
