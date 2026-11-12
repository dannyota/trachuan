// Package chinhphu obtains evidence from the government document site.
package chinhphu

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/cookiejar"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// ErrNotFound means the source has no requested document or file.
var ErrNotFound = errors.New("chinhphu: not found")

// ErrDeferred means the requested retry delay exceeds the allowed wait.
var ErrDeferred = errors.New("chinhphu: retry deferred")

// ErrTooLarge means a response exceeds its individual limit.
var ErrTooLarge = errors.New("chinhphu: response too large")

// ErrBudget means the instance has exhausted its response byte budget.
var ErrBudget = errors.New("chinhphu: download budget exceeded")

// Limits bounds requests and responses made by one shared client.
type Limits struct {
	RequestsPerSecond                     float64
	Burst, InFlight, Pages                int
	HTMLBytes, FileBytes                  int64
	Redirects                             int
	PageTimeout, FileTimeout, ItemTimeout time.Duration
}

// Gate admits requests and records source pauses.
type Gate interface {
	Admit(context.Context, int64, string) error
	Pause(context.Context, int64, string, string, string) error
}

// Client holds source session and request state. Share one instance per worker.
type Client struct {
	page, file   string
	testHosts    map[string]bool
	gate         Gate
	http         *http.Client
	transport    http.RoundTripper
	limits       Limits
	limiter      *rate.Limiter
	slots        chan struct{}
	backoffs     []time.Duration
	budget, used int64
	budgetMu     sync.Mutex
	session      chan struct{}
	now          func() time.Time
	sleep        func(context.Context, time.Duration) error
	resolve      func(context.Context, string) ([]net.IPAddr, error)
}

// Option configures a client.
type Option func(*Client)

// New creates a client with the source budgets.
func New(opts ...Option) *Client {
	jar, _ := cookiejar.New(nil)
	c := &Client{page: "https://vanban.chinhphu.vn", file: "https://datafiles.chinhphu.vn", testHosts: map[string]bool{}, http: &http.Client{Jar: jar}, limits: Limits{RequestsPerSecond: 1, Burst: 1, InFlight: 1, Pages: 100, HTMLBytes: 5000000, FileBytes: 50000000, Redirects: 3}, backoffs: []time.Duration{2 * time.Second, 8 * time.Second, 30 * time.Second}, budget: 1000000000, now: time.Now, sleep: wait, resolve: net.DefaultResolver.LookupIPAddr}
	for _, o := range opts {
		o(c)
	}
	if c.limits.RequestsPerSecond <= 0 {
		c.limits.RequestsPerSecond = 1
	}
	if c.limits.Burst <= 0 {
		c.limits.Burst = 1
	}
	if c.limits.InFlight <= 0 {
		c.limits.InFlight = 1
	}
	if c.limits.Pages <= 0 {
		c.limits.Pages = 100
	}
	if c.limits.HTMLBytes <= 0 {
		c.limits.HTMLBytes = 5000000
	}
	if c.limits.FileBytes <= 0 {
		c.limits.FileBytes = 50000000
	}
	if c.limits.PageTimeout <= 0 {
		c.limits.PageTimeout = 30 * time.Second
	}
	if c.limits.FileTimeout <= 0 {
		c.limits.FileTimeout = 120 * time.Second
	}
	if c.limits.ItemTimeout <= 0 {
		c.limits.ItemTimeout = 5 * time.Minute
	}
	c.session = make(chan struct{}, 1)
	c.limiter = rate.NewLimiter(rate.Limit(c.limits.RequestsPerSecond), c.limits.Burst)
	c.slots = make(chan struct{}, c.limits.InFlight)
	if c.transport == nil {
		c.transport = c.safeTransport()
	}
	c.http.Transport = c.transport
	c.http.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return c
}

// ItemTimeout returns the configured timeout for one item.
func (c *Client) ItemTimeout() time.Duration { return c.limits.ItemTimeout }

// WithBaseURLs sets local test endpoints and permits their exact origins.
func WithBaseURLs(page, file string) Option {
	return func(c *Client) { c.page = page; c.file = file; c.testHosts[page] = true; c.testHosts[file] = true }
}

// WithLimits sets request and response limits.
func WithLimits(l Limits) Option { return func(c *Client) { c.limits = l } }

// WithBackoffs sets retry delays.
func WithBackoffs(d ...time.Duration) Option {
	return func(c *Client) { c.backoffs = append([]time.Duration{}, d...) }
}

// WithDownloadBudget bounds all response bytes read by this instance.
func WithDownloadBudget(n int64) Option { return func(c *Client) { c.budget = n } }

// WithGate sets the shared source admission gate.
func WithGate(g Gate) Option { return func(c *Client) { c.gate = g } }

// WithTransport sets a transport for offline tests.
func WithTransport(t http.RoundTripper) Option { return func(c *Client) { c.transport = t } }

// WithResolver sets the resolver used to validate public destinations.
func WithResolver(r func(context.Context, string) ([]net.IPAddr, error)) Option {
	return func(c *Client) { c.resolve = r }
}

// WithClock sets the clock and cancellable sleep for retry tests.
func WithClock(now func() time.Time, sleep func(context.Context, time.Duration) error) Option {
	return func(c *Client) { c.now = now; c.sleep = sleep }
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
