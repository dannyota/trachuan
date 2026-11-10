package chinhphu

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"danny.vn/trachuan/apps/corpus/internal/lookup"
	"danny.vn/trachuan/apps/corpus/internal/source"
	xhtml "golang.org/x/net/html"
)

const prefix = "ctrl_191017_163$"

func (c *Client) fetchPage(ctx context.Context, method, target string, form url.Values) ([]byte, *xhtml.Node, error) {
	var b []byte
	var n *xhtml.Node
	e := c.request(ctx, method, target, form, false, func(r io.Reader, h http.Header) error {
		var err error
		b, err = c.readPage(r, h)
		if err != nil {
			return err
		}
		n, err = parseHTML(b)
		if err != nil {
			return err
		}
		if isChallenge(n, b) {
			return c.pause(ctx, "challenge")
		}
		c.form(n)
		b = c.redact(b)
		return nil
	})
	if e != nil {
		return nil, nil, e
	}
	return b, n, nil
}

// Search returns all observed keyword matches and redacted page evidence.
func (c *Client) Search(ctx context.Context, keyword string) (lookup.Result, [][]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, c.limits.ItemTimeout)
	defer cancel()
	select {
	case c.session <- struct{}{}:
		defer func() { <-c.session }()
	case <-ctx.Done():
		return lookup.Result{}, nil, fmt.Errorf("wait for search session: %w", ctx.Err())
	}
	query, _ := json.Marshal(map[string]any{"source": "chinhphu", "type": "keyword", "keyword": keyword, "cap": 500})
	b := lookup.Builder{QueryIdentity: source.KeywordSearchIdentity("chinhphu", keyword, 500), Query: query, Items: []lookup.Item{}, Pages: []lookup.Page{}}
	bodies := [][]byte{}
	_, home, e := c.fetchPage(ctx, http.MethodGet, c.page, nil)
	if e != nil {
		b.Failed = true
		return b.Result(), bodies, fmt.Errorf("search home: %w", e)
	}
	form := c.form(home)
	form.Set(prefix+"txtSearchKeyword", keyword)
	form.Set(prefix+"drdRecordPerPage", "500")
	form.Set(prefix+"drdDocOrg", "0")
	form.Set(prefix+"btnSearch", "Tìm kiếm")
	seen := map[string]bool{}
	previous := 0
	for page := 1; page <= c.limits.Pages; page++ {
		if page > 1 {
			form.Set("__EVENTTARGET", prefix+"grvDocument")
			form.Set("__EVENTARGUMENT", fmt.Sprintf("Page$%d", page))
			form.Del(prefix + "btnSearch")
		}
		raw, node, err := c.fetchPage(ctx, http.MethodPost, c.page, form)
		if err != nil {
			b.Failed = true
			return b.Result(), bodies, fmt.Errorf("search page: %w", err)
		}
		bodies = append(bodies, raw)
		form = c.form(node)
		form.Set(prefix+"txtSearchKeyword", keyword)
		form.Set(prefix+"drdRecordPerPage", "500")
		form.Set(prefix+"drdDocOrg", "0")
		safeNode, parseErr := parseHTML(raw)
		if parseErr != nil {
			b.Failed = true
			return b.Result(), bodies, fmt.Errorf("parse redacted search: %w", parseErr)
		}
		items, start, end, total, valid := parseSearch(safeNode)
		sum := sha256.Sum256(raw)
		hash := hex.EncodeToString(sum[:])
		ordinal := page
		record := lookup.Page{Ordinal: &ordinal, ResultIDs: []string{}, SHA256: hash, Path: fmt.Sprintf("page-%d.html", page)}
		if total >= 0 {
			record.Total = &total
			if b.TotalReported == nil {
				t := total
				b.TotalReported = &t
			}
		} else {
			valid = false
		}
		added := 0
		for _, item := range items {
			record.ResultIDs = append(record.ResultIDs, item.SourceID)
			if !seen[item.SourceID] {
				seen[item.SourceID] = true
				added++
				item.PageRef = hash
				b.Items = append(b.Items, item)
			}
		}
		b.Pages = append(b.Pages, record)
		if !valid || total >= 500 || start != previous+1 && total != 0 || end-start+1 != len(items) && total != 0 {
			b.Uncertain = true
		}
		if total == 0 || total >= 0 && end >= total {
			b.EndedNormally = true
			return b.Result(), bodies, nil
		}
		if !valid || added == 0 {
			b.Uncertain = true
			return b.Result(), bodies, nil
		}
		previous = end
	}
	b.Failed = true
	return b.Result(), bodies, fmt.Errorf("search page limit exceeded")
}

// LookupNumber returns all substring matches for a document number.
func (c *Client) LookupNumber(ctx context.Context, number string) (lookup.Result, [][]byte, error) {
	return c.Search(ctx, number)
}
func ptr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
func parseSearch(n *xhtml.Node) ([]lookup.Item, int, int, int, bool) {
	var grid, info *xhtml.Node
	walk(n, func(x *xhtml.Node) {
		if attr(x, "id") == "ctrl_191017_163_grvDocument" {
			grid = x
		}
		if attr(x, "id") == "document_page_info" {
			info = x
		}
	})
	items := []lookup.Item{}
	if grid == nil || info == nil {
		return items, 0, 0, -1, false
	}
	text := strings.TrimSpace(content(info))
	if text == "" && strings.Contains(content(grid), "Không có thông tin văn bản!") {
		return items, 0, 0, 0, true
	}
	m := regexp.MustCompile(`^\s*(\d+)\s*-\s*(\d+)\s*\|\s*(\d+)\s*$`).FindStringSubmatch(text)
	if m == nil {
		return items, 0, 0, -1, false
	}
	a, _ := strconv.Atoi(m[1])
	z, _ := strconv.Atoi(m[2])
	total, _ := strconv.Atoi(m[3])
	valid := a > 0 && z >= a && z <= total
	walk(grid, func(row *xhtml.Node) {
		if row.Type != xhtml.ElementNode || row.Data != "tr" {
			return
		}
		var id, number, title, date string
		walk(row, func(x *xhtml.Node) {
			if x.Type != xhtml.ElementNode {
				return
			}
			if x.Data == "a" && id == "" {
				u, e := url.Parse(attr(x, "href"))
				if e == nil {
					id = u.Query().Get("docid")
				}
			}
			if hasClass(x, "code") {
				number = content(x)
			}
			if hasClass(x, "substract") { //nolint:misspell // The source uses this CSS class.
				title = content(x)
			}
			if hasClass(x, "issued-date") && date == "" {
				date = content(x)
			}
		})
		if id != "" {
			if _, e := strconv.Atoi(id); e != nil {
				valid = false
				return
			}
			items = append(items, lookup.Item{Source: "chinhphu", SourceID: id, RawNumber: ptr(number), Title: ptr(title), IssueDate: ptr(date)})
		}
	})
	return items, a, z, total, valid
}
