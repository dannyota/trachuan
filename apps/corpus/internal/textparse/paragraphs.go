// Package textparse turns transcribed text into citable provisions.
package textparse

import (
	"fmt"
	"io"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/text/unicode/norm"
)

const maxParagraphs = 50000
const maxDepth = 64

// Paragraph is normalized text and its source locator.
type Paragraph struct {
	Text    string
	Ordinal int
	Locator int
	Table   bool
}

// LimitError reports a text extraction limit that was exceeded.
type LimitError struct {
	Limit   string
	Maximum int
}

func (e *LimitError) Error() string { return fmt.Sprintf("%s exceeds limit %d", e.Limit, e.Maximum) }

func normalize(s string) string { return norm.NFC.String(strings.Join(strings.Fields(s), " ")) }

// ParagraphsFromText returns one paragraph per nonempty OCR line.
func ParagraphsFromText(text string) ([]Paragraph, error) {
	var out []Paragraph
	for i, line := range strings.Split(text, "\n") {
		if s := normalize(line); s != "" {
			if len(out) == maxParagraphs {
				return nil, &LimitError{"paragraphs", maxParagraphs}
			}
			out = append(out, Paragraph{Text: s, Ordinal: len(out) + 1, Locator: i + 1})
		}
	}
	return out, nil
}

// ParagraphsFromHTML extracts blocks, table cells and image markers in order.
func ParagraphsFromHTML(data []byte) ([]Paragraph, error) {
	z := html.NewTokenizer(strings.NewReader(string(data)))
	var out []Paragraph
	var buf strings.Builder
	var stack []string
	cellDepth, hidden := 0, 0
	flush := func() error {
		s := normalize(buf.String())
		buf.Reset()
		if s == "" {
			return nil
		}
		if len(out) == maxParagraphs {
			return &LimitError{"paragraphs", maxParagraphs}
		}
		out = append(out, Paragraph{Text: s, Ordinal: len(out) + 1, Locator: len(out) + 1, Table: cellDepth > 0})
		return nil
	}
	for {
		tt := z.Next()
		switch tt {
		case html.ErrorToken:
			if z.Err() != io.EOF {
				return nil, fmt.Errorf("tokenize HTML: %w", z.Err())
			}
			if err := flush(); err != nil {
				return nil, err
			}
			return out, nil
		case html.TextToken:
			if hidden == 0 {
				buf.Write(z.Text())
			}
		case html.StartTagToken, html.SelfClosingTagToken:
			name, _ := z.TagName()
			tag := string(name)
			if isBlock(tag) && (cellDepth == 0 || tag == "td" || tag == "th" || tag == "table") {
				if err := flush(); err != nil {
					return nil, err
				}
			}
			if isBlock(tag) && cellDepth > 0 && tag != "td" && tag != "th" && tag != "table" {
				buf.WriteByte(' ')
			}
			if tag == "img" || tag == "math" {
				if err := flush(); err != nil {
					return nil, err
				}
				buf.WriteString("[hình]")
				if err := flush(); err != nil {
					return nil, err
				}
			}
			if tag == "br" {
				buf.WriteByte(' ')
			}
			if tt == html.StartTagToken && !isVoid(tag) {
				stack = append(stack, tag)
				if len(stack) > maxDepth {
					return nil, &LimitError{"nesting depth", maxDepth}
				}
				if tag == "td" || tag == "th" {
					cellDepth++
				}
				if tag == "script" || tag == "style" || tag == "math" {
					hidden++
				}
			}
		case html.EndTagToken:
			name, _ := z.TagName()
			tag := string(name)
			if isBlock(tag) && (cellDepth == 0 || tag == "td" || tag == "th" || tag == "table") {
				if err := flush(); err != nil {
					return nil, err
				}
			}
			for i := len(stack) - 1; i >= 0; i-- {
				if stack[i] == tag {
					for _, v := range stack[i:] {
						if v == "td" || v == "th" {
							cellDepth--
						}
						if v == "script" || v == "style" || v == "math" {
							hidden--
						}
					}
					stack = stack[:i]
					break
				}
			}
		}
	}
}
func isBlock(s string) bool {
	switch s {
	case "p", "div", "section", "article", "header", "footer", "h1", "h2", "h3", "h4", "h5", "h6", "li", "td", "th", "tr", "table", "blockquote", "pre":
		return true
	}
	return false
}
func isVoid(s string) bool {
	switch s {
	case "area", "base", "br", "col", "embed", "hr", "img", "input", "link", "meta", "param", "source", "track", "wbr":
		return true
	}
	return false
}
