package ocrcheck

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

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
