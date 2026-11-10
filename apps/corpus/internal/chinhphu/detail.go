package chinhphu

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	xhtml "golang.org/x/net/html"
)

// Attachment describes a source-linked file.
type Attachment struct{ Name, URL string }

// Detail holds the fields and attachment inventory displayed by the source.
type Detail struct {
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
	q := u.Query()
	q.Set("pageid", "27160")
	q.Set("docid", fmt.Sprint(id))
	u.RawQuery = q.Encode()
	raw, _, e := c.fetchPage(ctx, http.MethodGet, u.String(), nil)
	if e != nil {
		return Detail{}, nil, fmt.Errorf("detail: %w", e)
	}
	node, e := parseHTML(raw)
	if e != nil {
		return Detail{}, raw, fmt.Errorf("parse redacted detail: %w", e)
	}
	d := Detail{URL: u.String(), Fields: map[string]string{}, Attachments: []Attachment{}}
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
			return Detail{}, raw, ErrNotFound
		}
		return Detail{}, raw, &Failure{Origin: "source_data", Cause: "invalid_payload", Endpoint: "detail"}
	}
	invalidAttachment := false
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
			return
		}
		key := strings.TrimSpace(content(cells[0]))
		value := strings.TrimSpace(content(cells[1]))
		if key == "" {
			return
		}
		if key != "Tài liệu đính kèm" {
			d.Fields[key] = value
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
			d.Attachments = append(d.Attachments, Attachment{Name: strings.TrimSpace(content(a)), URL: href})
		})
	})
	if invalidAttachment {
		return Detail{}, raw, &Failure{Origin: "source_data", Cause: "invalid_payload", Endpoint: "detail", Err: errInvalidPayload}
	}
	d.Number = d.Fields["Số ký hiệu"]
	d.Title = d.Fields["Trích yếu"]
	d.Issuer = d.Fields["Cơ quan ban hành"]
	d.IssueDate = d.Fields["Ngày ban hành"]
	return d, raw, nil
}
