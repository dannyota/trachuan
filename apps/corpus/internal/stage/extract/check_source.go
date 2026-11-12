package extract

import (
	"context"
	"fmt"

	"danny.vn/trachuan/apps/corpus/internal/store"
)

// CheckSource identifies the OCR entry selected by the ordinary text-source rules.
type CheckSource struct {
	Entry   store.OcrText
	PDFName string
}

// SelectCheckSources returns every accepted engine entry for the selected OCR
// PDF. Extract continues to select one entry by engine order.
func SelectCheckSources(ctx context.Context, dir, key string, source ocrReader, engines []string, checker string) ([]CheckSource, error) {
	preferred, err := SelectCheckSource(ctx, dir, key, source, engines, checker)
	if err != nil || preferred == nil {
		return nil, err
	}
	var result []CheckSource
	seen := map[int64]bool{}
	for _, engine := range engines {
		entries, lookupErr := source.Lookup(ctx, []string{preferred.Entry.PdfSha256}, []string{engine}, checker)
		if lookupErr != nil {
			return nil, fmt.Errorf("lookup OCR check entries: %w", lookupErr)
		}
		if entry, ok := entries[preferred.Entry.PdfSha256]; ok && !seen[entry.Text.ID] {
			seen[entry.Text.ID] = true
			result = append(result, CheckSource{Entry: entry.Text, PDFName: preferred.PDFName})
		}
	}
	return result, nil
}

// SelectCheckSource reuses extract's selection without requiring a completed
// check. It does not freeze a run selection or change any artifact.
func SelectCheckSource(ctx context.Context, dir, key string, source ocrReader, engines []string, checker string) (*CheckSource, error) {
	selected, _, err := chooseForCheck(ctx, dir, key, source, engines, checker, true)
	if err != nil {
		return nil, err
	}
	if selected.source != "ocr" || selected.entry == nil {
		return nil, nil
	}
	return &CheckSource{Entry: selected.entry.Text, PDFName: selected.name}, nil
}
