package extract

import (
	"context"

	"danny.vn/trachuan/apps/corpus/internal/store"
)

// CheckSource identifies the OCR entry selected by the ordinary text-source rules.
type CheckSource struct {
	Entry   store.OcrText
	PDFName string
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
