package chinhphu

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strings"

	xhtml "golang.org/x/net/html"
)

// Attachment describes a source-linked file.
type Attachment struct{ Name, Label, URL string }

// FieldOccurrence holds one displayed field and its untrimmed text.
type FieldOccurrence struct {
	Name, Value string
	Cells       []string
	Ambiguous   bool
}

// Detail holds the fields and attachment inventory displayed by the source.
type Detail struct {
	Occurrences                           []FieldOccurrence
	Number, Title, Issuer, IssueDate, URL string
	Fields                                map[string]string
	Attachments                           []Attachment
}

// Detail retrieves a document's displayed metadata and redacted HTML.
func (c *Client) Detail(ctx context.Context, id int) (Detail, []byte, error) {
	ctx, cancel := context.WithTimeout(ctx, c.limits.ItemTimeout)
	defer cancel()
	if id <= 0 {
		return Detail{}, nil, fmt.Errorf("invalid document id")
	}
	u, e := url.Parse(c.page)
	if e != nil {
		return Detail{}, nil, fmt.Errorf("invalid base URL: %w", e)
	}
	q := url.Values{}
	q.Set("pageid", "27160")
	q.Set("docid", fmt.Sprint(id))
	u.RawQuery = q.Encode()
	u.Fragment = ""
	raw, _, e := c.fetchPage(ctx, http.MethodGet, u.String(), nil)
	if e != nil {
		return Detail{}, nil, fmt.Errorf("detail: %w", e)
	}
	d, e := c.ParseDetail(raw, u.String())
	return d, raw, e
}

// ParseDetail parses a saved detail response without making a request.
func (c *Client) ParseDetail(raw []byte, sourceURL string) (Detail, error) {
	node, e := parseHTML(raw)
	if e != nil {
		return Detail{}, fmt.Errorf("parse redacted detail: %w", e)
	}
	d := Detail{URL: sourceURL, Fields: map[string]string{}, Attachments: []Attachment{}}
	var container *xhtml.Node
	walk(node, func(n *xhtml.Node) {
		if attr(n, "id") == "ctrl_190596_91_Content" {
			container = n
		}
	})
	hasTable := false
	if container != nil {
		walk(container, func(n *xhtml.Node) {
			if n.Type == xhtml.ElementNode && n.Data == "table" {
				hasTable = true
			}
		})
	}
	if !hasTable {
		if strings.Contains(content(node), "Không tìm thấy văn bản này") {
			return Detail{}, ErrNotFound
		}
		return Detail{}, &Failure{Origin: "source_data", Cause: "invalid_payload", Endpoint: "detail"}
	}
	invalidAttachment := false
	conflicts := map[string]bool{}
	walk(container, func(row *xhtml.Node) {
		if row.Type != xhtml.ElementNode || row.Data != "tr" {
			return
		}
		var cells []*xhtml.Node
		for n := row.FirstChild; n != nil; n = n.NextSibling {
			if n.Type == xhtml.ElementNode && n.Data == "td" {
				cells = append(cells, n)
			}
		}
		if len(cells) != 2 {
			if len(cells) == 0 {
				return
			}
			key := strings.TrimSpace(content(cells[0]))
			if key == "" {
				return
			}
			texts := make([]string, 0, len(cells)-1)
			for _, cell := range cells[1:] {
				texts = append(texts, content(cell))
			}
			d.Occurrences = append(d.Occurrences, FieldOccurrence{Name: content(cells[0]), Cells: texts, Ambiguous: true})
			conflicts[key] = true
			delete(d.Fields, key)
			return
		}
		key := strings.TrimSpace(content(cells[0]))
		rawValue := content(cells[1])
		value := strings.TrimSpace(rawValue)
		if key == "" {
			return
		}
		d.Occurrences = append(d.Occurrences, FieldOccurrence{Name: content(cells[0]), Value: rawValue})
		if key != "Tài liệu đính kèm" {
			if prior, ok := d.Fields[key]; ok && prior != value {
				conflicts[key] = true
			}
			if !conflicts[key] {
				d.Fields[key] = value
			} else {
				delete(d.Fields, key)
			}
			return
		}
		walk(cells[1], func(a *xhtml.Node) {
			if a.Type != xhtml.ElementNode || a.Data != "a" {
				return
			}
			href := attr(a, "href")
			if strings.HasPrefix(strings.ToLower(strings.TrimSpace(href)), "javascript:") {
				return
			}
			v, err := url.Parse(href)
			if err != nil || v.User != nil || v.Fragment != "" {
				invalidAttachment = true
				return
			}
			origin := v.Scheme + "://" + v.Host
			testFile := c.testHosts[origin] && origin == c.file && (v.Scheme == "http" || v.Scheme == "https")
			official := v.Scheme == "https" && strings.EqualFold(v.Hostname(), "datafiles.chinhphu.vn") && (v.Port() == "" || v.Port() == "443")
			if !testFile && !official {
				invalidAttachment = true
				return
			}
			d.Attachments = append(d.Attachments, Attachment{Name: path.Base(v.Path), Label: content(a), URL: href})
		})
	})
	if invalidAttachment {
		return Detail{}, &Failure{Origin: "source_data", Cause: "invalid_payload", Endpoint: "detail", Err: errInvalidPayload}
	}
	d.Number = d.Fields["Số ký hiệu"]
	d.Title = d.Fields["Trích yếu"]
	d.Issuer = d.Fields["Cơ quan ban hành"]
	d.IssueDate = d.Fields["Ngày ban hành"]
	return d, nil
}
