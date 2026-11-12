package ocrcheck

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"unicode/utf8"

	"danny.vn/trachuan/apps/corpus/internal/ocrstore"
	"golang.org/x/text/unicode/norm"
)

func collapsed(s string) string { return strings.Join(strings.Fields(norm.NFC.String(s)), " ") }

// decodeAnswer reports only fixed rule names and known schema paths.
func decodeAnswer(data []byte, pages int, text []byte) (ocrstore.CheckResult, string) {
	var empty ocrstore.CheckResult
	if _, err := ocrstore.DecodeAnswer(data, pages); errors.Is(err, ocrstore.ErrNUL) {
		return empty, "schema:nul"
	}
	root, reason := answerObject(data, "", []string{"same_document", "pages", "items"})
	if reason != "" {
		return empty, reason
	}
	var same bool
	var readPages int
	var items []json.RawMessage
	for _, field := range []struct {
		name string
		out  any
	}{{"same_document", &same}, {"pages", &readPages}, {"items", &items}} {
		if err := json.Unmarshal(root[field.name], field.out); err != nil {
			return empty, "schema:" + field.name
		}
	}
	if readPages < 1 {
		return empty, "schema:pages"
	}
	if items == nil {
		return empty, "schema:items"
	}
	decoded := make([]ocrstore.CheckItem, 0, len(items))
	for i, raw := range items {
		path := fmt.Sprintf("items[%d]", i)
		fields, itemReason := answerObject(raw, path, []string{"kind", "page", "ocr", "pdf", "note"})
		if itemReason != "" {
			return empty, itemReason
		}
		var item ocrstore.CheckItem
		for _, field := range []struct {
			name string
			out  any
		}{{"kind", &item.Kind}, {"page", &item.Page}, {"ocr", &item.OCR}, {"pdf", &item.PDF}, {"note", &item.Note}} {
			if err := json.Unmarshal(fields[field.name], field.out); err != nil {
				return empty, "schema:" + path + "." + field.name
			}
		}
		if item.Page < 1 || item.Page > readPages {
			return empty, "schema:" + path + ".page"
		}
		switch item.Kind {
		case "document_number", "date", "article_label", "amount", "missing_text", "extra_text":
		default:
			return empty, "schema:" + path + ".kind"
		}
		decoded = append(decoded, item)
	}
	if readPages != pages {
		return empty, "page_count_mismatch"
	}
	checked := make([]ocrstore.CheckItem, 0, len(decoded))
	dropped := 0
	source := collapsed(string(text))
	for _, item := range decoded {
		// Validate every quote, even if the item would subsequently be dropped.
		quote := collapsed(item.OCR)
		reading := collapsed(item.PDF)
		valid := quote != "" && reading != ""
		switch item.Kind {
		case "extra_text":
			valid = quote != "" && reading == ""
		case "missing_text":
			valid = quote == "" && reading != ""
		}
		if !valid {
			return empty, "item_readings"
		}
		if quote != "" && !strings.Contains(source, quote) {
			return empty, "ocr_quote_not_found"
		}
		if item.Kind != "missing_text" && quote == reading {
			dropped++
			continue
		}
		checked = append(checked, item)
	}
	canonical, err := json.Marshal(struct {
		Same  bool                 `json:"same_document"`
		Pages int                  `json:"pages"`
		Items []ocrstore.CheckItem `json:"items"`
	}{same, readPages, checked})
	if err != nil {
		return empty, "schema:json"
	}
	result, err := ocrstore.DecodeAnswer(canonical, pages)
	if err != nil {
		return empty, "schema:validated_answer"
	}
	result.DroppedEqual = dropped
	return result, ""
}

func answerObject(data []byte, path string, fields []string) (map[string]json.RawMessage, string) {
	rule := func(name string) string {
		if path != "" {
			return "schema:" + path + ":" + name
		}
		return "schema:" + name
	}
	fieldPath := func(name string) string {
		if path != "" {
			return "schema:" + path + "." + name
		}
		return "schema:" + name
	}
	d := json.NewDecoder(bytes.NewReader(data))
	token, err := d.Token()
	if err != nil || token != json.Delim('{') {
		return nil, rule("object")
	}
	values := make(map[string]json.RawMessage, len(fields))
	for d.More() {
		token, err = d.Token()
		if err != nil {
			return nil, rule("json")
		}
		key, ok := token.(string)
		if !ok || !slices.Contains(fields, key) {
			return nil, rule("unknown_field")
		}
		if _, exists := values[key]; exists {
			return nil, fieldPath(key) + ":duplicate"
		}
		var value json.RawMessage
		if err = d.Decode(&value); err != nil {
			return nil, fieldPath(key) + ":json"
		}
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return nil, fieldPath(key) + ":null"
		}
		values[key] = value
	}
	if _, err = d.Token(); err != nil {
		return nil, rule("json")
	}
	if _, err = d.Token(); err != io.EOF {
		return nil, rule("trailing_data")
	}
	for _, key := range fields {
		if _, ok := values[key]; !ok {
			return nil, fieldPath(key) + ":missing"
		}
	}
	return values, ""
}

// answerDiagnostic records only safe facts from a complete JSON object.
func answerDiagnostic(data []byte, u *Usage) {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(data, &root); err != nil || root == nil {
		return
	}
	if raw, ok := root["pages"]; ok {
		var value any
		d := json.NewDecoder(bytes.NewReader(raw))
		d.UseNumber()
		if d.Decode(&value) == nil {
			switch value.(type) {
			case json.Number:
				var n int
				if json.Unmarshal(raw, &n) == nil {
					u.AnswerPages = &n
				} else {
					u.AnswerPagesKind = "number"
				}
			case string:
				u.AnswerPagesKind = "string"
			case []any:
				u.AnswerPagesKind = "array"
			case map[string]any:
				u.AnswerPagesKind = "object"
			case bool:
				u.AnswerPagesKind = "bool"
			case nil:
				u.AnswerPagesKind = "null"
			}
		}
	}
	if raw, ok := root["same_document"]; ok && !bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		var same bool
		if json.Unmarshal(raw, &same) == nil {
			u.AnswerSameDocument = &same
		}
	}
	if raw, ok := root["items"]; ok {
		var items []json.RawMessage
		if json.Unmarshal(raw, &items) == nil && items != nil {
			n := len(items)
			u.AnswerItems = &n
			invalid := 0
			for _, item := range items {
				var fields map[string]json.RawMessage
				var page int
				validPage := json.Unmarshal(item, &fields) == nil && fields != nil && len(fields["page"]) != 0 && !bytes.Equal(bytes.TrimSpace(fields["page"]), []byte("null")) && json.Unmarshal(fields["page"], &page) == nil
				if validPage && (u.AnswerItemPageMax == nil || page > *u.AnswerItemPageMax) {
					value := page
					u.AnswerItemPageMax = &value
				}
				if !validPage || page < 1 || (u.AnswerPages != nil && page > *u.AnswerPages) {
					invalid++
					if len(u.AnswerInvalidItems) < 10 {
						u.AnswerInvalidItems = append(u.AnswerInvalidItems, invalidItemDiagnostic(fields))
					}
				}
			}
			u.AnswerItemPageInvalid = &invalid
		}
	}
}

func invalidItemDiagnostic(fields map[string]json.RawMessage) InvalidItemDiagnostic {
	kind := "other"
	var value string
	if json.Unmarshal(fields["kind"], &value) == nil {
		switch value {
		case "document_number", "date", "article_label", "amount", "missing_text", "extra_text":
			kind = value
		}
	}
	chars := func(name string) *int {
		var text *string
		if json.Unmarshal(fields[name], &text) != nil || text == nil {
			return nil
		}
		n := utf8.RuneCountInString(*text)
		return &n
	}
	return InvalidItemDiagnostic{Kind: kind, OCRChars: chars("ocr"), PDFChars: chars("pdf"), NoteChars: chars("note")}
}
