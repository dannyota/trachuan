package vbpl

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"syscall"
	"time"

	"golang.org/x/time/rate"
)

// ErrNotFound means the gateway did not find the requested document.
var ErrNotFound = errors.New("vbpl: not found")

// ErrTooLarge means a response exceeds its size limit.
var ErrTooLarge = errors.New("vbpl: response too large")

const apiLimit int64 = 20_000_000

// Client limits gateway requests and downloads.
type Client struct {
	base, userAgent string
	http            *http.Client
	limiter         *rate.Limiter
	slots           chan struct{}
	timeout         time.Duration
	backoffs        []time.Duration
	fileHosts       []string
	allowTestHTTP   bool
}

// New creates a client identifying this release by version.
func New(version string) *Client {
	return NewWithBase("https://vbpl-bientap-gateway.moj.gov.vn/api/qtdc/public", version)
}

// NewWithBase creates a client for the supplied gateway URL.
func NewWithBase(base, version string, options ...Option) *Client {
	c := &Client{base: base, userAgent: "traquydinh/" + version + " (+https://github.com/dannyota/traquydinh)", http: &http.Client{}, limiter: rate.NewLimiter(8, 1), slots: make(chan struct{}, 8), timeout: 30 * time.Second, fileHosts: []string{"s3-han02.fptcloud.com"}, backoffs: []time.Duration{2 * time.Second, 8 * time.Second, 30 * time.Second}}
	for _, option := range options {
		option(c)
	}
	c.http.Transport = &http.Transport{Proxy: http.ProxyFromEnvironment, TLSNextProto: map[string]func(string, *tls.Conn) http.RoundTripper{}, ForceAttemptHTTP2: false, MaxConnsPerHost: cap(c.slots), MaxIdleConnsPerHost: cap(c.slots)}
	c.http.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return c
}

// Option configures a client.
type Option func(*Client)

// WithLimits sets the request rate and maximum concurrent requests.
func WithLimits(rps float64, inFlight int) Option {
	return func(c *Client) {
		c.limiter = rate.NewLimiter(rate.Limit(rps), 1)
		c.slots = make(chan struct{}, inFlight)
	}
}

// WithBackoffs sets the delay before each retry.
func WithBackoffs(delays ...time.Duration) Option {
	return func(c *Client) { c.backoffs = append([]time.Duration(nil), delays...) }
}

// WithFileHosts sets the permitted attachment hosts.
func WithFileHosts(hosts ...string) Option {
	return func(c *Client) { c.fileHosts = append([]string(nil), hosts...) }
}

// WithTestHTTP permits HTTP downloads from 127.0.0.1 for local tests.
func WithTestHTTP() Option {
	return func(c *Client) { c.allowTestHTTP = true }
}

type response struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data"`
}

// request holds a concurrency slot and timeout until the response body is consumed.
func (c *Client) request(ctx context.Context, method, target string, body []byte, consume func(io.Reader) error) (int, error) {
	select {
	case c.slots <- struct{}{}:
	case <-ctx.Done():
		return 0, fmt.Errorf("wait for request slot: %w", ctx.Err())
	}
	defer func() { <-c.slots }()
	if err := c.limiter.Wait(ctx); err != nil {
		return 0, fmt.Errorf("wait for rate limit: %w", err)
	}
	reqCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, method, target, bytes.NewReader(body))
	if err != nil {
		return 0, fmt.Errorf("create request: %w", safeError(err))
	}
	req.Header.Set("Origin", "https://vbpl.vn")
	req.Header.Set("Referer", "https://vbpl.vn/")
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, fmt.Errorf("request: %w", safeError(err))
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return resp.StatusCode, nil
	}
	if err := consume(resp.Body); err != nil {
		return resp.StatusCode, fmt.Errorf("read response: %w", safeError(err))
	}
	return resp.StatusCode, nil
}
func safeError(err error) error {
	var u *url.Error
	if errors.As(err, &u) {
		return u.Err
	}
	return err
}
func retryable(status int, err error) bool {
	var n net.Error
	return status == 429 || status >= 500 || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, syscall.ECONNRESET) || (errors.As(err, &n) && n.Timeout())
}
func wait(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
func (c *Client) apiRaw(ctx context.Context, method, path string, body []byte) (json.RawMessage, error) {
	for attempt := 0; ; attempt++ {
		if attempt > 0 {
			if err := wait(ctx, c.backoffs[attempt-1]); err != nil {
				return nil, fmt.Errorf("retry: %w", err)
			}
		}
		var raw []byte
		status, err := c.request(ctx, method, c.base+path, body, func(r io.Reader) error {
			var e error
			raw, e = io.ReadAll(io.LimitReader(r, apiLimit+1))
			if e != nil {
				return e
			}
			if int64(len(raw)) > apiLimit {
				return ErrTooLarge
			}
			return nil
		})
		if retryable(status, err) && attempt < len(c.backoffs) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("gateway request: %w", err)
		}
		if status == 404 {
			return nil, ErrNotFound
		}
		if status != 200 {
			return nil, fmt.Errorf("gateway status %d", status)
		}
		return raw, nil
	}
}
func (c *Client) api(ctx context.Context, method, path string, body []byte) (json.RawMessage, error) {
	raw, err := c.apiRaw(ctx, method, path, body)
	if err != nil {
		return nil, err
	}
	var envelope response
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, fmt.Errorf("decode gateway response: %w", err)
	}
	if !envelope.Success {
		return nil, ErrNotFound
	}
	return raw, nil
}

func data(raw json.RawMessage, out any) error {
	var env response
	if err := json.Unmarshal(raw, &env); err != nil {
		return fmt.Errorf("decode envelope: %w", err)
	}
	if err := json.Unmarshal(env.Data, out); err != nil {
		return fmt.Errorf("decode data: %w", err)
	}
	return nil
}
func docPath(id string) string { return "/doc/" + url.PathEscape(id) }

// List requests one page of a discovery query.
func (c *Client) List(ctx context.Context, q ListQuery) (ListPage, error) {
	input := struct {
		Page      int      `json:"pageNumber"`
		Size      int      `json:"pageSize"`
		Sort      string   `json:"sortBy"`
		Direction string   `json:"sortDirection"`
		Group     bool     `json:"groupVbpl"`
		Level     string   `json:"agencyLevel"`
		Option    string   `json:"optionDoc"`
		Mode      string   `json:"matchMode"`
		Agencies  []string `json:"agencyIds,omitempty"`
		Keyword   string   `json:"keyword,omitempty"`
	}{q.Page, 50, "issueDate", "desc", false, "TRUNG_UONG", "title", "all_words", q.AgencyIDs, q.Keyword}
	body, err := json.Marshal(input)
	if err != nil {
		return ListPage{}, fmt.Errorf("encode list query: %w", err)
	}
	raw, err := c.api(ctx, http.MethodPost, "/doc/all", body)
	if err != nil {
		return ListPage{}, err
	}
	var result struct {
		Total int               `json:"total"`
		Items []json.RawMessage `json:"items"`
	}
	if err := data(raw, &result); err != nil {
		return ListPage{}, err
	}
	page := ListPage{Total: result.Total}
	for _, item := range result.Items {
		var v struct{ ID, DocNum, Title, IssueDate string }
		if err := json.Unmarshal(item, &v); err != nil {
			return ListPage{}, fmt.Errorf("decode list item: %w", err)
		}
		page.Items = append(page.Items, ListItem{v.ID, v.DocNum, v.Title, v.IssueDate, item})
	}
	return page, nil
}

// Detail requests raw and decoded document metadata.
func (c *Client) Detail(ctx context.Context, id string) (json.RawMessage, Detail, error) {
	raw, err := c.api(ctx, http.MethodGet, docPath(id), nil)
	if err != nil {
		return nil, Detail{}, err
	}
	var v struct {
		ID, DocNum, Title, AgencyName, IssueDate, EffFrom, EffTo string
		DocType, EffStatus                                       struct{ Code, Name string }
		IsConsolidatedDocument, HasContent                       bool
		DocumentContent                                          struct{ Content string }
		References                                               []json.RawMessage
	}
	if err := data(raw, &v); err != nil {
		return nil, Detail{}, err
	}
	d := Detail{ID: v.ID, DocNum: v.DocNum, Title: v.Title, DocType: v.DocType.Name, Agency: v.AgencyName, IssueDate: v.IssueDate, EffFrom: v.EffFrom, EffTo: v.EffTo, StatusCode: v.EffStatus.Code, StatusName: v.EffStatus.Name, IsConsolidated: v.IsConsolidatedDocument, HasContent: v.HasContent || v.DocumentContent.Content != ""}
	for _, ref := range v.References {
		var r struct {
			ReferenceType  json.RawMessage
			TargetDocument struct {
				ID            json.RawMessage
				DocNum, Title string
			}
		}
		if err := json.Unmarshal(ref, &r); err != nil {
			return nil, Detail{}, fmt.Errorf("decode reference: %w", err)
		}
		d.References = append(d.References, Reference{TargetID: jsonText(r.TargetDocument.ID), TargetNumber: r.TargetDocument.DocNum, Code: jsonText(r.ReferenceType), Label: r.TargetDocument.Title, Raw: ref})
	}
	return raw, d, nil
}
func jsonText(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	if string(raw) == "null" {
		return ""
	}
	return string(raw)
}

// RawTree requests the unparsed provision tree response.
func (c *Client) RawTree(ctx context.Context, id string) (json.RawMessage, error) {
	return c.apiRaw(ctx, http.MethodGet, "/doc/provision/tree/"+url.PathEscape(id), nil)
}

// Tree requests raw and decoded provision nodes.
func (c *Client) Tree(ctx context.Context, id string) (json.RawMessage, []Node, error) {
	raw, err := c.RawTree(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	nodes, err := DecodeTree(ctx, bytes.NewReader(raw), MaxTreeDepth, MaxTreeNodes)
	if err != nil {
		return nil, nil, err
	}
	return raw, nodes, nil
}

// Diagram requests the raw relation diagram.
func (c *Client) Diagram(ctx context.Context, id string) (json.RawMessage, error) {
	return c.api(ctx, http.MethodGet, docPath(id)+"/diagram", nil)
}

// Files requests fresh attachment URLs.
func (c *Client) Files(ctx context.Context, id string) ([]File, error) {
	raw, err := c.api(ctx, http.MethodGet, "/doc/minio/buckets/vbpl/folders/"+url.PathEscape(id)+"/files", nil)
	if err != nil {
		return nil, err
	}
	var entries []struct {
		FileName, FullPath, PresignedURL, UpdatedAt string
		Size                                        int64
	}
	if err := data(raw, &entries); err != nil {
		return nil, err
	}
	files := make([]File, 0, len(entries))
	for _, e := range entries {
		updated, _ := time.ParseInLocation("2006-01-02 15:04:05", e.UpdatedAt, time.FixedZone("ICT", 7*60*60))
		files = append(files, File{Name: e.FileName, FullPath: e.FullPath, Size: e.Size, URL: e.PresignedURL, UpdatedAt: updated})
	}
	return files, nil
}
