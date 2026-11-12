// Package ocrvision reads public PDF page images with Google Cloud Vision.
package ocrvision

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/time/rate"
)

const visionURL = "https://vision.googleapis.com"

// EngineID identifies the fresh engine and its rendering settings.
const EngineID = "vision-ocr/builtin-latest/poppler-200-jpeg"

// Config supplies authentication and test seams. Production uses the fixed URL.
type Config struct {
	Project      string
	TokenSource  oauth2.TokenSource
	BaseURL      string
	HTTP         *http.Client
	Sleep        func(context.Context, time.Duration) error
	Wait         func(context.Context) error
	TokenTimeout time.Duration
}

// Client sends one page per request without logging request or response content.
type Client struct {
	project, baseURL string
	tokens           oauth2.TokenSource
	http             *http.Client
	sleep            func(context.Context, time.Duration) error
	wait             func(context.Context) error
	tokenTimeout     time.Duration
}

// NewClient constructs an invocation-scoped client with a 300 request/minute cap.
func NewClient(cfg Config) *Client {
	if cfg.BaseURL == "" {
		cfg.BaseURL = visionURL
	}
	transport := directHTTPClient(cfg.HTTP)
	if cfg.Sleep == nil {
		cfg.Sleep = sleep
	}
	if cfg.Wait == nil {
		cfg.Wait = rate.NewLimiter(rate.Every(time.Minute/600), 1).Wait
	}
	return &Client{project: cfg.Project, baseURL: cfg.BaseURL, tokens: cfg.TokenSource, http: transport, sleep: cfg.Sleep, wait: cfg.Wait, tokenTimeout: cfg.TokenTimeout}
}

func sleep(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// Preflight refuses unusable credentials before any page is rendered.
func (c *Client) Preflight(ctx context.Context) error {
	if c == nil || c.project == "" || c.tokens == nil {
		return fmt.Errorf("GOOGLE_APPLICATION_CREDENTIALS and GOOGLE_CLOUD_PROJECT are required")
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("vision authentication canceled: %w", err)
	}
	token, err := c.token(ctx)
	if err != nil || token == nil || !token.Valid() {
		return fmt.Errorf("GOOGLE_APPLICATION_CREDENTIALS must provide usable credentials; GOOGLE_CLOUD_PROJECT is required")
	}
	return nil
}

type pageRequest struct {
	Image struct {
		Content string `json:"content"`
	} `json:"image"`
	Features     []feature `json:"features"`
	ImageContext struct {
		LanguageHints []string `json:"languageHints"`
	} `json:"imageContext"`
}
type feature struct {
	Type  string `json:"type"`
	Model string `json:"model"`
}
type pageResponse struct {
	FullTextAnnotation struct {
		Text string `json:"text"`
	} `json:"fullTextAnnotation"`
	Error *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

type requestError struct {
	code      int
	transient bool
}

func (e requestError) Error() string {
	return fmt.Sprintf("Vision code=%d message=page request failed", e.code)
}

func (c *Client) recognize(ctx context.Context, png []byte, b *budget) (string, error) {
	for attempt := 0; attempt < 4; attempt++ {
		text, err := c.attempt(ctx, png, b)
		if err == nil {
			return text, nil
		}
		failure, ok := err.(requestError)
		if !ok || !failure.transient || attempt == 3 || b.remaining() == 0 {
			return "", err
		}
		if err = c.sleep(ctx, []time.Duration{time.Second, 4 * time.Second, 16 * time.Second}[attempt]); err != nil {
			return "", fmt.Errorf("vision retry canceled: %w", err)
		}
	}
	return "", requestError{}
}

func (c *Client) attempt(ctx context.Context, png []byte, b *budget) (string, error) {
	if b.remaining() == 0 {
		return "", requestError{}
	}
	ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()
	if err := c.wait(ctx); err != nil {
		return "", requestError{}
	}
	token, err := c.token(ctx)
	if err != nil || token == nil || !token.Valid() {
		return "", requestError{}
	}
	page := pageRequest{Features: []feature{{Type: "DOCUMENT_TEXT_DETECTION", Model: "builtin/latest"}}}
	page.Image.Content = base64.StdEncoding.EncodeToString(png)
	page.ImageContext.LanguageHints = []string{"vi"}
	data, err := json.Marshal(struct {
		Requests []pageRequest `json:"requests"`
	}{[]pageRequest{page}})
	if err != nil {
		return "", requestError{}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/images:annotate", bytes.NewReader(data))
	if err != nil {
		return "", requestError{}
	}
	// The HTTP/2 transport must return refusals to our budgeted retry loop.
	req.GetBody = nil
	req.Header.Set("Authorization", "Bearer "+token.AccessToken)
	req.Header.Set("x-goog-user-project", c.project)
	req.Header.Set("Content-Type", "application/json")
	if ctx.Err() != nil || !b.debit() {
		return "", requestError{}
	}
	response, err := c.http.Do(req)
	if err != nil {
		return "", requestError{transient: true}
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		transient := false
		switch response.StatusCode {
		case 429, 500, 502, 503, 504:
			transient = true
		}
		return "", requestError{code: response.StatusCode, transient: transient}
	}
	// Legal text is bounded like stored OCR text. Never expose the body in errors.
	body, err := io.ReadAll(io.LimitReader(response.Body, 50_000_001))
	if err != nil {
		return "", requestError{transient: true}
	}
	if len(body) > 50_000_000 {
		return "", requestError{}
	}
	var result struct {
		Responses []pageResponse `json:"responses"`
	}
	if json.Unmarshal(body, &result) != nil || len(result.Responses) != 1 {
		return "", requestError{}
	}
	r := result.Responses[0]
	if r.Error != nil {
		transient := false
		switch r.Error.Code {
		case 4, 8, 13, 14:
			transient = true
		}
		return "", requestError{code: r.Error.Code, transient: transient}
	}
	return r.FullTextAnnotation.Text, nil
}
