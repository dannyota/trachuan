package extract

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"danny.vn/trachuan/apps/corpus/internal/ocrstore"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/stage/fetch"
	"danny.vn/trachuan/apps/corpus/internal/store"
	"danny.vn/trachuan/apps/corpus/internal/textparse"
)

type ocrReader interface {
	Lookup(context.Context, []string, []string, string) (map[string]ocrstore.Entry, error)
	ReadText(store.OcrText) ([]byte, error)
}

func selectOCRFiles(ctx context.Context, dir, id string, files []fetch.DocumentFile, source ocrReader, engines []string, checker string) (selection, error) {
	return selectOCRFilesMode(ctx, dir, id, files, source, engines, checker, false)
}

func selectOCRFilesMode(ctx context.Context, dir, id string, files []fetch.DocumentFile, source ocrReader, engines []string, checker string, unchecked bool) (selection, error) {
	selected, err := selectFiles(dir, id, files, absentOCR{})
	if err != nil || selected.source != "none" {
		return selected, err
	}
	names := map[string]string{}
	var hashes []string
	for _, f := range files {
		if !strings.EqualFold(filepath.Ext(f.Name), ".pdf") {
			continue
		}
		if _, ok := names[f.SHA256]; !ok {
			names[f.SHA256] = f.Name
			hashes = append(hashes, f.SHA256)
		}
	}
	if len(hashes) == 0 {
		return selected, nil
	}
	selected.consulted = true
	if source == nil {
		return selected, nil
	}
	entries, err := source.Lookup(ctx, hashes, engines, checker)
	if err != nil {
		return selection{}, fmt.Errorf("lookup OCR text: %w", err)
	}
	hash := ""
	if len(hashes) == 1 {
		hash = hashes[0]
	} else if len(entries) == 1 {
		for h := range entries {
			hash = h
		}
	} else {
		return selection{source: "none", gap: "text_source_ambiguous", consulted: true}, nil
	}
	entry, found := entries[hash]
	if !found || (entry.Check == nil && !unchecked) {
		return selection{source: "none", gap: "needs_ocr", consulted: true}, nil
	}
	if unchecked {
		return selection{source: "ocr", name: names[hash], hash: hash, entry: &entry, consulted: true}, nil
	}
	result, err := ocrstore.DecodeCheck(entry.Check.Result)
	if err != nil {
		return selection{}, fmt.Errorf("invalid stored OCR check: %w", err)
	}
	if result.Verdict == "rejected" {
		return selection{source: "none", gap: "ocr_rejected", reason: "checker rejected OCR text", consulted: true}, nil
	}
	if result.Verdict != "clean" && result.Verdict != "flagged" {
		return selection{}, fmt.Errorf("invalid stored OCR verdict")
	}
	return selection{source: "ocr", name: names[hash], hash: hash, entry: &entry, consulted: true}, nil
}

func selectionInputHash(fetchHash string, selected selection) string {
	if selected.source == "ocr" && selected.entry != nil && selected.entry.Check != nil {
		return fetch.ExtractInputHash(fetchHash, "ocr", selected.entry.Text.TextSha256, selected.entry.Check.ResultSha256)
	}
	if selected.source == "none" {
		return fetch.ExtractInputHash(fetchHash, "none", selected.gap)
	}
	return InputHash(fetchHash, selected.source)
}

func parseOCR(selected selection, data []byte) ([]provision, []stage.Gap, *textRecord) {
	entry := selected.entry
	paragraphs, err := textparse.ParagraphsFromText(string(data))
	flagged := entry.Check.Flagged
	record := &textRecord{TextSource: "ocr", FileName: selected.name, SHA256: selected.hash, EngineID: entry.Text.EngineID, TextSHA256: entry.Text.TextSha256, CheckerID: entry.Check.CheckerID, ResultSHA256: entry.Check.ResultSha256, Flagged: &flagged, Paragraphs: paragraphs}
	rows, gaps, record := parseParagraphs(paragraphs, err, record)
	for i := range record.Provisions {
		p := &record.Provisions[i]
		if p.StartOrdinal > 0 && p.EndOrdinal <= len(paragraphs) && p.EndOrdinal >= p.StartOrdinal {
			p.StartLine = paragraphs[p.StartOrdinal-1].Locator
			p.EndLine = paragraphs[p.EndOrdinal-1].Locator
		}
	}
	if flagged > 0 {
		gaps = append(gaps, stage.Gap{Kind: "ocr_flagged"})
	}
	return rows, gaps, record
}
