package ocrstore

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/jackc/pgx/v5/pgtype"
)

type checkItem struct {
	Kind *string `json:"kind"`
	Page *int    `json:"page"`
	OCR  *string `json:"ocr"`
	PDF  *string `json:"pdf"`
	Note *string `json:"note"`
}
type checkResult struct {
	SameDocument *bool        `json:"same_document"`
	Pages        *int         `json:"pages"`
	Items        *[]checkItem `json:"items"`
	Verdict      *string      `json:"verdict"`
}

func validateCheck(data []byte, flagged int32, pages pgtype.Int4) ([]byte, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var result checkResult
	if err := decoder.Decode(&result); err != nil {
		return nil, fmt.Errorf("invalid check result: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("invalid check result trailing data")
	}
	if result.SameDocument == nil || result.Pages == nil || result.Items == nil || result.Verdict == nil || *result.Pages < 1 || (pages.Valid && *result.Pages != int(pages.Int32)) || int64(len(*result.Items)) != int64(flagged) {
		return nil, fmt.Errorf("invalid check coverage or count")
	}
	missing := map[int]bool{}
	for _, item := range *result.Items {
		if item.Kind == nil || item.Page == nil || item.OCR == nil || item.PDF == nil || item.Note == nil || *item.Page < 1 || *item.Page > *result.Pages {
			return nil, fmt.Errorf("invalid check item")
		}
		switch *item.Kind {
		case "document_number", "date", "article_label", "amount", "extra_text":
		case "missing_text":
			missing[*item.Page] = true
		default:
			return nil, fmt.Errorf("invalid check item kind")
		}
	}
	expected := "clean"
	if !*result.SameDocument || len(missing)*10 > *result.Pages {
		expected = "rejected"
	} else if len(*result.Items) > 0 {
		expected = "flagged"
	}
	if *result.Verdict != expected {
		return nil, fmt.Errorf("invalid check verdict")
	}
	var object map[string]any
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	if err := dec.Decode(&object); err != nil {
		return nil, fmt.Errorf("decode check: %w", err)
	}
	canonical, err := json.Marshal(object)
	if err != nil {
		return nil, fmt.Errorf("encode check: %w", err)
	}
	return canonical, nil
}
