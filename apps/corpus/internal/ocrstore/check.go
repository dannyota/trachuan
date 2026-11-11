package ocrstore

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"slices"

	"github.com/jackc/pgx/v5/pgtype"
)

// CheckItem is one disagreement with a PDF page.
type CheckItem struct {
	Kind string `json:"kind"`
	Page int    `json:"page"`
	OCR  string `json:"ocr"`
	PDF  string `json:"pdf"`
	Note string `json:"note"`
}

// CheckResult is the validated evidence stored beside immutable OCR text.
type CheckResult struct {
	SameDocument bool        `json:"same_document"`
	Pages        int         `json:"pages"`
	PDFPages     int         `json:"pdf_pages"`
	Items        []CheckItem `json:"items"`
	Verdict      string      `json:"verdict"`
	DroppedEqual int         `json:"dropped_equal"`
}

// DecodeAnswer validates the model schema and derives the verdict locally.
func DecodeAnswer(data []byte, pdfPages int) (CheckResult, error) {
	return decodeResult(data, pdfPages, false)
}

// DecodeCheck validates a stored result, including its locally derived verdict.
func DecodeCheck(data []byte) (CheckResult, error) { return decodeResult(data, 0, true) }

func decodeResult(data []byte, pdfPages int, stored bool) (CheckResult, error) {
	fields := []string{"same_document", "pages", "items"}
	if stored {
		fields = append(fields, "pdf_pages", "verdict")
		// Old immutable results predate this local counter. Preserve their readability.
		var existing map[string]json.RawMessage
		if json.Unmarshal(data, &existing) == nil {
			if _, ok := existing["dropped_equal"]; ok {
				fields = append(fields, "dropped_equal")
			}
		}
	}
	obj, err := strictObject(data, fields)
	if err != nil {
		return CheckResult{}, err
	}
	var result CheckResult
	// Required fields and exact spelling were checked before typed decoding.
	if err = json.Unmarshal(data, &result); err != nil {
		return CheckResult{}, fmt.Errorf("invalid check types")
	}
	if result.Pages < 1 || result.Items == nil || result.DroppedEqual < 0 {
		return CheckResult{}, fmt.Errorf("invalid check coverage")
	}
	var items []json.RawMessage
	if err = json.Unmarshal(obj["items"], &items); err != nil {
		return CheckResult{}, fmt.Errorf("invalid check items")
	}
	missing := map[int]bool{}
	for i, item := range items {
		if _, err = strictObject(item, []string{"kind", "page", "ocr", "pdf", "note"}); err != nil {
			return CheckResult{}, err
		}
		v := result.Items[i]
		if v.Page < 1 || v.Page > result.Pages {
			return CheckResult{}, fmt.Errorf("invalid check page")
		}
		switch v.Kind {
		case "document_number", "date", "article_label", "amount", "extra_text":
		case "missing_text":
			missing[v.Page] = true
		default:
			return CheckResult{}, fmt.Errorf("invalid check kind")
		}
	}
	verdict := "clean"
	if !result.SameDocument || len(missing)*10 > result.Pages {
		verdict = "rejected"
	} else if len(items) > 0 {
		verdict = "flagged"
	}
	if stored {
		if result.PDFPages < 1 || result.Pages != result.PDFPages || result.Verdict != verdict {
			return CheckResult{}, fmt.Errorf("invalid stored check coverage or verdict")
		}
	} else {
		result.PDFPages = pdfPages
		result.Verdict = verdict
	}
	return result, nil
}

func strictObject(data []byte, fields []string) (map[string]json.RawMessage, error) {
	d := json.NewDecoder(bytes.NewReader(data))
	token, err := d.Token()
	if err != nil || token != json.Delim('{') {
		return nil, fmt.Errorf("invalid check object")
	}
	values := make(map[string]json.RawMessage, len(fields))
	for d.More() {
		token, err = d.Token()
		if err != nil {
			return nil, fmt.Errorf("invalid check field")
		}
		key, ok := token.(string)
		if !ok || !slices.Contains(fields, key) {
			return nil, fmt.Errorf("unknown check field")
		}
		if _, found := values[key]; found {
			return nil, fmt.Errorf("repeated check field")
		}
		var value json.RawMessage
		if err = d.Decode(&value); err != nil || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return nil, fmt.Errorf("invalid check value")
		}
		values[key] = value
	}
	if _, err = d.Token(); err != nil {
		return nil, fmt.Errorf("invalid check object end")
	}
	if _, err = d.Token(); err != io.EOF {
		return nil, fmt.Errorf("trailing check data")
	}
	if len(values) != len(fields) {
		return nil, fmt.Errorf("missing check field")
	}
	return values, nil
}

func validateCheck(data []byte, flagged int32, pages pgtype.Int4) ([]byte, error) {
	result, err := DecodeCheck(data)
	if err != nil {
		return nil, err
	}
	if int64(len(result.Items)) != int64(flagged) || (pages.Valid && result.Pages != int(pages.Int32)) {
		return nil, fmt.Errorf("invalid check coverage or count")
	}
	canonical, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("encode check: %w", err)
	}
	return canonical, nil
}
