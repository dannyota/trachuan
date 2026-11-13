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

// ErrDeferred means Retry-After exceeds ten minutes.
var ErrDeferred = errors.New("vbpl: retry deferred")

// ErrTooLarge means a response exceeds its size limit.
var ErrTooLarge = errors.New("vbpl: response too large")

const apiLimit int64 = 20_000_000

// gatewayAttemptTimeout allows the gateway's 30-second 504 to arrive. See docs/design/budgets.md.
const gatewayAttemptTimeout = 40 * time.Second

// Client limits gateway requests and downloads.
type Client struct {
	base           string
	http           *http.Client
	fileHTTP       *http.Client
	transport      http.RoundTripper
	limiter        *rate.Limiter
	slots          chan struct{}
	fileSlots      chan struct{}
	timeout        time.Duration
	fileTimeout    time.Duration
	requestBudget  time.Duration
	now            func() time.Time
	sleep          func(context.Context, time.Duration) error
	slowThreshold  time.Duration
	backoffs       []time.Duration
	fileHosts      []string
	allowTestHTTP  bool
	gate           sourcegate.Gate
	lookupPageSize int
	pauses         sourcegate.Pauses
}

// New creates a client. The version argument is retained for compatibility.
func New(version string) *Client {
	return NewWithBase("https://vbpl-bientap-gateway.moj.gov.vn/api/qtdc/public", version)
}

// NewWithBase creates a client for the supplied gateway URL. The version argument is ignored.
func NewWithBase(base, _ string, options ...Option) *Client {
	c := &Client{lookupPageSize: 1000, base: base, http: &http.Client{}, limiter: rate.NewLimiter(16, 1), slots: make(chan struct{}, 16), fileSlots: make(chan struct{}, 8), timeout: gatewayAttemptTimeout, fileTimeout: 30 * time.Second, fileHosts: []string{"s3-han02.fptcloud.com"}, backoffs: []time.Duration{2 * time.Second, 8 * time.Second, 30 * time.Second}}
	c.requestBudget = 200 * time.Second
	c.now, c.sleep = time.Now, wait
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
	if c.transport != nil {
		c.http.Transport, c.fileHTTP.Transport = c.transport, c.transport
	}
	return c
}

// Option configures a client.
type Option func(*Client)

// WithTransport supplies the transport for gateway and attachment requests.
func WithTransport(transport http.RoundTripper) Option {
	return func(c *Client) { c.transport = transport }
}

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

// WithTestTimeouts sets short request deadlines for offline tests.
func WithTestTimeouts(attempt, sequence time.Duration) Option {
	return func(c *Client) { c.timeout, c.fileTimeout, c.requestBudget = attempt, attempt, sequence }
}

type response struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data"`
}

// request holds a concurrency slot and timeout until the response body is consumed.
func (c *Client) request(ctx context.Context, method, target string, body []byte, file bool, opts requestOptions, consume func(io.Reader) error) (status int, err error) {
	started := time.Now()
	defer func() {
		sourcegate.LogSlow(ctx, "vbpl", endpoint(target), time.Since(started), c.slowThreshold, status, err)
	}()
	timeout := c.timeout
	if file {
		timeout = c.fileTimeout
	}
	if opts.timeout > 0 {
		timeout = opts.timeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
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
			if ctx.Err() == nil {
				waitErr = fmt.Errorf("%w: %v", context.DeadlineExceeded, waitErr)
			}
			return 0, fmt.Errorf("wait for rate limit: %w", waitErr)
		}
	}
	if c.gate != nil {
		id, ok := sourcegate.RunID(ctx)
		if !ok {
			return 0, fmt.Errorf("source admission: missing run id")
		}
		if pauseErr := c.awaitPause(ctx, id); pauseErr != nil {
			return 0, pauseErr
		}
		if admitErr := c.gate.Admit(ctx, id, "vbpl"); admitErr != nil {
			return 0, fmt.Errorf("source admission: %w", admitErr)
		}
		if pauseErr := c.awaitPause(ctx, id); pauseErr != nil {
			return 0, pauseErr
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, target, bytes.NewReader(body))
	if err != nil {
		return 0, fmt.Errorf("create request: %w", safeError(err))
	}
	req.Header.Set("Origin", "https://vbpl.vn")
	req.Header.Set("Referer", "https://vbpl.vn/")
	req.Header.Set("User-Agent", sourcegate.UserAgent)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if opts.ranged {
		req.Header.Set("Range", "bytes=0-0")
	}
	if opts.started != nil {
		*opts.started = time.Now()
	}
	resp, err := client.Do(req)
	if err != nil {
		classified := classify(safeError(err), endpoint(target), 0)
		var failure *Failure
		if errors.As(classified, &failure) {
			return 0, classified
		}
		return 0, &Failure{Origin: "transport", Cause: "connection_error", Endpoint: endpoint(target), Err: safeError(err)}
	}
	defer func() { _ = resp.Body.Close() }()
	if opts.headers != nil {
		*opts.headers = resp.Header.Clone()
	}
	successStatus := http.StatusOK
	if opts.ranged {
		successStatus = http.StatusPartialContent
	}
	head, reader := sourcegate.ResponsePrefix(ctx, responseBody{reader: resp.Body, endpoint: endpoint(target), status: resp.StatusCode}, resp.Body, resp.StatusCode, successStatus, challenge)
	if challenge(head) {
		return resp.StatusCode, c.deny(ctx, endpoint(target)+" challenge")
	}
	if resp.StatusCode == http.StatusForbidden {
		return resp.StatusCode, c.deny(ctx, endpoint(target)+" status 403")
	}
	if resp.StatusCode != successStatus {
		return resp.StatusCode, nil
	}
	if err := consume(reader); err != nil {
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
	var f *Failure
	return errors.As(err, &f) && f.Origin == "transport" || status == 429 || status >= 500 && status <= 599 || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, syscall.ECONNRESET) || (errors.As(err, &n) && n.Timeout())
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
		return nil, &Failure{Origin: "gateway", Cause: "unknown", Endpoint: endpoint(path), HTTPStatus: 200, Err: ErrNotFound}
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
func (c *Client) Detail(ctx context.Context, id string) (raw json.RawMessage, detail Detail, err error) {
	ctx, finish := sourcegate.WithBodyValidation(ctx)
	defer func() { finish(invalidJSONFields(err)) }()
	raw, err = c.api(ctx, http.MethodGet, docPath(id), nil)
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
func (c *Client) Diagram(ctx context.Context, id string) (raw json.RawMessage, err error) {
	ctx, finish := sourcegate.WithBodyValidation(ctx)
	defer func() { finish(invalidJSONFields(err)) }()
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

type requestOptions struct {
	timeout time.Duration
	ranged  bool
	headers *http.Header
	started *time.Time
}

// WithClock sets the retry clock and cancellable wait for offline tests.
func WithClock(now func() time.Time, sleep func(context.Context, time.Duration) error) Option {
	return func(c *Client) { c.now, c.sleep = now, sleep }
}
