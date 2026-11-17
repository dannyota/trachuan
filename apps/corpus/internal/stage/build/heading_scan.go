package build

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"danny.vn/trachuan/apps/corpus/internal/source"

	"github.com/jackc/pgx/v5"
	"golang.org/x/text/unicode/norm"
)

type summaryReader interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

func scanReasons(heading string) []string {
	text := norm.NFC.String(heading)
	reasons := []string{}
	if utf8.RuneCountInString(text) > 200 {
		reasons = append(reasons, "over_200_characters")
	}
	last, _ := utf8.DecodeLastRuneInString(strings.TrimSpace(text))
	if strings.ContainsRune(".!?…。", last) {
		reasons = append(reasons, "sentence_final_mark")
	}
	words := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) && !unicode.IsMark(r) })
	for _, phrase := range []string{"phải", "được", "không được", "có trách nhiệm"} {
		parts := strings.Fields(phrase)
		for i := 0; i+len(parts) <= len(words); i++ {
			if slices.Equal(words[i:i+len(parts)], parts) {
				reasons = append(reasons, "words:"+phrase)
				break
			}
		}
	}
	return reasons
}

func encodeHeadingScan(cc, rel string, documents []summaryDocument, omissions []headingOmission) ([]byte, error) {
	type candidate struct {
		Key     string   `json:"key"`
		Anchor  string   `json:"anchor"`
		Heading string   `json:"heading"`
		Reasons []string `json:"reasons"`
	}
	slices.SortFunc(omissions, func(a, b headingOmission) int {
		if a.Length != b.Length {
			return cmp.Compare(b.Length, a.Length)
		}
		if a.Key != b.Key {
			return strings.Compare(a.Key, b.Key)
		}
		return strings.Compare(a.Anchor, b.Anchor)
	})
	type omittedHeadings struct {
		Count   int               `json:"count"`
		Longest []headingOmission `json:"longest"`
	}
	items := []candidate{}
	slices.SortFunc(documents, func(a, b summaryDocument) int { return strings.Compare(a.Key, b.Key) })
	for _, d := range documents {
		for _, n := range d.Payload.Outline {
			if n.Heading == "" {
				continue
			}
			if reasons := scanReasons(n.Heading); len(reasons) > 0 {
				items = append(items, candidate{d.Key, n.Anchor, n.Heading, reasons})
			}
		}
	}
	result := struct {
		Country    string          `json:"country"`
		Release    string          `json:"release"`
		Projection int             `json:"projection"`
		Matching   string          `json:"matching"`
		Items      []candidate     `json:"items"`
		Omitted    omittedHeadings `json:"rule_3_omitted"`
	}{cc, rel, 1, "NFC; Unicode lowercase; words are maximal Unicode letter, mark or number runs; punctuation separates phrase words; final marks: . ! ? … 。", items, omittedHeadings{len(omissions), append([]headingOmission{}, omissions[:min(20, len(omissions))]...)}}
	b, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("encode heading scan: %w", err)
	}
	return append(b, '\n'), nil
}

func HeadingScan(ctx context.Context, db summaryReader, cc, rel string, exclusions []source.HeadingExclusion) ([]byte, error) {
	rows, err := db.Query(ctx, `SELECT document_key,payload FROM document_summary WHERE projection=1 ORDER BY document_key COLLATE "C"`)
	if err != nil {
		return nil, fmt.Errorf("query stored candidate headings: %w", err)
	}
	defer rows.Close()
	documents := []summaryDocument{}
	for rows.Next() {
		var d summaryDocument
		var raw []byte
		if err = rows.Scan(&d.Key, &raw); err != nil {
			return nil, fmt.Errorf("scan stored candidate headings: %w", err)
		}
		if err = json.Unmarshal(raw, &d.Payload); err != nil {
			return nil, fmt.Errorf("decode stored summary: %w", err)
		}
		documents = append(documents, d)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("read stored candidate headings: %w", err)
	}
	omissions, err := scanHeadingOmissions(ctx, db, exclusions)
	if err != nil {
		return nil, err
	}
	return encodeHeadingScan(cc, rel, documents, omissions)
}
