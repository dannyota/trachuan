package ocrcheck

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"danny.vn/trachuan/apps/corpus/internal/ocrstore"
	"danny.vn/trachuan/apps/corpus/internal/store"
	"danny.vn/trachuan/apps/corpus/internal/textparse"
)

// ReviewItem adds provision anchors to a disagreement without changing it.
type ReviewItem struct {
	ocrstore.CheckItem
	Anchors []string `json:"anchors"`
}

// ReviewDocument contains the evidence a person needs to inspect a check.
type ReviewDocument struct {
	DocumentKey string       `json:"document_key"`
	PDFName     string       `json:"pdf_name"`
	PDFSHA256   string       `json:"pdf_sha256"`
	EngineID    string       `json:"engine_id"`
	CheckerID   string       `json:"checker_id"`
	Verdict     string       `json:"verdict"`
	Items       []ReviewItem `json:"items"`
}

// Review maps each OCR reading to all provisions that contain it.
func Review(key, pdfName string, entry store.OcrText, checker string, result ocrstore.CheckResult, text []byte) (ReviewDocument, error) {
	doc := ReviewDocument{DocumentKey: key, PDFName: pdfName, PDFSHA256: entry.PdfSha256, EngineID: entry.EngineID, CheckerID: checker, Verdict: result.Verdict, Items: []ReviewItem{}}
	paragraphs, err := textparse.ParagraphsFromText(string(text))
	if err != nil {
		return doc, fmt.Errorf("parse OCR review text: %w", err)
	}
	provisions := textparse.Parse(paragraphs).Provisions
	for _, item := range result.Items {
		v := ReviewItem{CheckItem: item, Anchors: []string{}}
		reading := strings.Join(strings.Fields(item.OCR), " ")
		if reading != "" {
			for _, p := range provisions {
				if strings.Contains(strings.Join(strings.Fields(p.Text), " "), reading) {
					v.Anchors = append(v.Anchors, p.Anchor)
				}
			}
		}
		doc.Items = append(doc.Items, v)
	}
	return doc, nil
}

// WriteReview writes the requested review artifact with private file permissions.
func WriteReview(path string, docs []ReviewDocument) error {
	data, err := json.MarshalIndent(docs, "", "  ")
	if err != nil {
		return fmt.Errorf("encode OCR review: %w", err)
	}
	if err = os.WriteFile(path, append(data, '\n'), 0600); err != nil {
		return fmt.Errorf("write OCR review: %w", err)
	}
	return nil
}
