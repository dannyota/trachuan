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

	"danny.vn/trachuan/apps/corpus/internal/sourcegate"
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
	fileHTTP        *http.Client
	limiter         *rate.Limiter
	slots           chan struct{}
	fileSlots       chan struct{}
	timeout         time.Duration
	requestBudget   time.Duration
	slowThreshold   time.Duration
	backoffs        []time.Duration
	fileHosts       []string
	allowTestHTTP   bool
	gate            sourcegate.Gate
	lookupPageSize  int
}

// New creates a client identifying this release by version.
func New(version string) *Client {
	return NewWithBase("https://vbpl-bientap-gateway.moj.gov.vn/api/qtdc/public", version)
}

// NewWithBase creates a client for the supplied gateway URL.
func NewWithBase(base, version string, options ...Option) *Client {
	c := &Client{lookupPageSize: 1000, base: base, userAgent: "trachuan/" + version + " (+https://github.com/dannyota/trachuan)", http: &http.Client{}, limiter: rate.NewLimiter(16, 1), slots: make(chan struct{}, 16), fileSlots: make(chan struct{}, 8), timeout: 30 * time.Second, fileHosts: []string{"s3-han02.fptcloud.com"}, backoffs: []time.Duration{2 * time.Second, 8 * time.Second, 30 * time.Second}}
	c.requestBudget = 200 * time.Second
	c.slowThreshold = 10 * time.Second
	for _, option := range options {
		option(c)
	}
	transport := func(limit int) *http.Transport {
		return &http.Transport{Proxy: http.ProxyFromEnvironment, TLSNextProto: map[string]func(string, *tls.Conn) http.RoundTripper{}, ForceAttemptHTTP2: false, MaxConnsPerHost: limit, MaxIdleConnsPerHost: limit}
	}
	redirect := func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	c.http.Transport = transport(cap(c.slots))
	c.http.CheckRedirect = redirect
	c.fileHTTP = &http.Client{Transport: transport(cap(c.fileSlots)), CheckRedirect: redirect}
	return c
}

// Option configures a client.
type Option func(*Client)

// WithGate enables source admission for every request.
func WithGate(g sourcegate.Gate) Option { return func(c *Client) { c.gate = g } }

// WithLimits sets the request rate and maximum concurrent requests.
func WithLimits(rps float64, inFlight int) Option {
	return func(c *Client) {
		c.limiter = rate.NewLimiter(rate.Limit(rps), 1)
		c.slots = make(chan struct{}, inFlight)
	}
}

// WithFileInFlight sets the maximum concurrent attachment requests.
func WithFileInFlight(inFlight int) Option {
	return func(c *Client) { c.fileSlots = make(chan struct{}, inFlight) }
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
func (c *Client) request(ctx context.Context, method, target string, body []byte, file bool, consume func(io.Reader) error) (status int, err error) {
	started := time.Now()
	defer func() {
		sourcegate.LogSlow(ctx, "vbpl", endpoint(target), time.Since(started), c.slowThreshold, status, err)
	}()
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	slots, client := c.slots, c.http
	if file {
		slots, client = c.fileSlots, c.fileHTTP
	}
	select {
	case slots <- struct{}{}:
	case <-ctx.Done():
		return 0, fmt.Errorf("wait for request slot: %w", ctx.Err())
	}
	defer func() { <-slots }()
	if !file {
		if waitErr := c.limiter.Wait(ctx); waitErr != nil {
			return 0, fmt.Errorf("wait for rate limit: %w", waitErr)
		}
	}
	if c.gate != nil {
		id, ok := sourcegate.RunID(ctx)
		if !ok {
			return 0, fmt.Errorf("source admission: missing run id")
		}
		if admitErr := c.gate.Admit(ctx, id, "vbpl"); admitErr != nil {
			return 0, fmt.Errorf("source admission: %w", admitErr)
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, target, bytes.NewReader(body))
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
	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("request: %w", safeError(err))
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusForbidden && c.gate != nil {
		id, _ := sourcegate.RunID(ctx)
		reason := fmt.Sprintf("%s status 403", endpoint(target))
		if err := c.gate.Pause(ctx, id, "vbpl", reason, ""); err != nil {
			return resp.StatusCode, fmt.Errorf("pause source: %w", err)
		}
		return resp.StatusCode, fmt.Errorf("%s: %w", reason, sourcegate.ErrSourcePaused)
	}
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
	ctx, cancel := context.WithTimeout(ctx, c.requestBudget)
	defer cancel()
	for attempt := 0; ; attempt++ {
		if attempt > 0 {
			if err := wait(ctx, c.backoffs[attempt-1]); err != nil {
				return nil, fmt.Errorf("retry: %w", err)
			}
		}
		var raw []byte
		status, err := c.request(ctx, method, c.base+path, body, false, func(r io.Reader) error {
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
		if err == nil && status == http.StatusOK && !objectFramed(raw) {
			err = &Failure{Origin: "gateway", Cause: "non_json_200", Endpoint: endpoint(path), HTTPStatus: status}
		}
		if (retryable(status, err) || isNonJSON(err)) && attempt < len(c.backoffs) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("gateway request: %w", classify(err, endpoint(path), status))
		}
		if status == 404 {
			return nil, ErrNotFound
		}
		if status != 200 {
			statusErr := &StatusError{Code: status, Endpoint: endpoint(path)}
			if status == 429 || status >= 500 {
				return nil, &Failure{Origin: "gateway", Cause: statusCause(status), Endpoint: endpoint(path), HTTPStatus: status, Err: statusErr}
			}
			return nil, statusErr
		}
		return raw, nil
	}
}

// StatusError reports a gateway response other than 200 or 404 that survived
// the client's retries.
type StatusError struct {
	Code     int
	Endpoint string
}

// Error describes the status.
func (e *StatusError) Error() string { return fmt.Sprintf("gateway status %d", e.Code) }

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
