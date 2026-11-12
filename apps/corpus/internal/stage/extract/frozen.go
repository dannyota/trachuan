package extract

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"danny.vn/trachuan/apps/corpus/internal/ocrstore"
	"danny.vn/trachuan/apps/corpus/internal/stage/fetch"
	"danny.vn/trachuan/apps/corpus/internal/store"
	"danny.vn/trachuan/apps/corpus/internal/vbpl"
	"github.com/jackc/pgx/v5"
)

type selectionQueries interface {
	GetRunOCRSelection(context.Context, store.GetRunOCRSelectionParams) (store.GetRunOCRSelectionRow, error)
	InsertRunOCRSelection(context.Context, store.InsertRunOCRSelectionParams) error
}

func planFrozenSelection(ctx context.Context, q selectionQueries, runID int64, key string, choose func() (selection, error)) (selection, error) {
	params := store.GetRunOCRSelectionParams{RunID: runID, ItemKey: key}
	row, err := q.GetRunOCRSelection(ctx, params)
	if err == nil {
		return selectionFromRow(row)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return selection{}, fmt.Errorf("read OCR selection: %w", err)
	}
	selected, err := choose()
	if err != nil {
		return selection{}, err
	}
	if !selected.consulted {
		return selected, nil
	}
	insert := store.InsertRunOCRSelectionParams{RunID: runID, ItemKey: key, Gap: selected.gap}
	if selected.source == "ocr" && selected.entry != nil && selected.entry.Check != nil {
		insert.PdfSha256 = &selected.hash
		insert.OcrTextID = &selected.entry.Text.ID
		insert.OcrCheckID = &selected.entry.Check.ID
	}
	if err = q.InsertRunOCRSelection(ctx, insert); err != nil {
		return selection{}, fmt.Errorf("record OCR selection: %w", err)
	}
	row, err = q.GetRunOCRSelection(ctx, params)
	if err != nil {
		return selection{}, fmt.Errorf("read recorded OCR selection: %w", err)
	}
	return selectionFromRow(row)
}

func selectionFromRow(row store.GetRunOCRSelectionRow) (selection, error) {
	if row.Gap != "" {
		switch row.Gap {
		case "tree_headings_only":
			return selection{source: "source_tree", gap: row.Gap, consulted: true}, nil
		case "needs_ocr", "text_source_ambiguous", "file_format_unsupported", "no_text_source", "ocr_rejected", "ocr_label_flagged":
			selected := selection{source: "none", gap: row.Gap, consulted: true}
			switch row.Gap {
			case "ocr_rejected":
				selected.reason = "checker rejected OCR text"
			case "ocr_label_flagged":
				selected.reason = "checker flagged an article label or document number"
			}
			return selected, nil
		default:
			return selection{}, fmt.Errorf("invalid frozen OCR gap")
		}
	}
	if row.PdfSha256 == nil || row.OcrTextID == nil || row.OcrCheckID == nil || row.EngineID == nil || row.TextSha256 == nil || row.CheckerID == nil || row.ResultSha256 == nil || !row.Flagged.Valid {
		return selection{}, fmt.Errorf("invalid frozen OCR selection")
	}
	entry := ocrstore.Entry{Text: store.OcrText{ID: *row.OcrTextID, PdfSha256: *row.PdfSha256, EngineID: *row.EngineID, TextSha256: *row.TextSha256}, Check: &store.OcrCheck{ID: *row.OcrCheckID, CheckerID: *row.CheckerID, ResultSha256: *row.ResultSha256, Flagged: row.Flagged.Int32}}
	return selection{source: "ocr", hash: *row.PdfSha256, entry: &entry, consulted: true}, nil
}

func materializeFrozen(ctx context.Context, dir, key string, selected selection) (selection, []vbpl.Node, error) {
	if selected.gap == "tree_headings_only" {
		fallback, nodes, err := choose(ctx, dir, key)
		if err != nil {
			return selection{}, nil, err
		}
		if fallback.source != "source_tree" || fallback.gap != "tree_headings_only" {
			return selection{}, nil, fmt.Errorf("invalid frozen heading-only tree")
		}
		return selected, nodes, nil
	}
	if selected.source == "ocr" {
		doc, err := fetch.ReadDocument(dir, key)
		if err != nil {
			return selection{}, nil, fmt.Errorf("read frozen OCR document: %w", err)
		}
		for _, f := range doc.Files {
			if f.SHA256 == selected.hash && strings.EqualFold(filepath.Ext(f.Name), ".pdf") {
				selected.name = f.Name
				return selected, nil, nil
			}
		}
		return selection{}, nil, fmt.Errorf("frozen OCR PDF not found in bundle")
	}
	return selected, nil, nil
}
