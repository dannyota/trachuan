package embedding

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type Report struct {
	ProtocolVersion int    `json:"protocol_version"`
	RecipeHash      string `json:"recipe_hash"`
	Recipe          Recipe `json:"recipe"`
}

type Client struct {
	base string
	http *http.Client
}

func NewClient(baseURL string, hc *http.Client) *Client {
	if hc == nil {
		hc = &http.Client{Timeout: 125 * time.Second}
	}
	return &Client{base: strings.TrimRight(baseURL, "/"), http: hc}
}

func (c *Client) request(ctx context.Context, path string, body any, out any) error {
	var reader io.Reader
	method := http.MethodGet
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode request: %w", err)
		}
		reader = bytes.NewReader(b)
		method = http.MethodPost
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, reader)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("embedder request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	dec := json.NewDecoder(io.LimitReader(resp.Body, 64<<20))
	if resp.StatusCode != http.StatusOK {
		var data struct {
			Error struct {
				Code       string `json:"code"`
				Index      *int   `json:"index"`
				TokenCount *int   `json:"token_count"`
			} `json:"error"`
		}
		if err := dec.Decode(&data); err != nil {
			return fmt.Errorf("decode embedder error: %w", err)
		}
		var retryAfter *int
		header := resp.Header.Get("Retry-After")
		if header != "" && strings.Trim(header, "0123456789") == "" {
			if seconds, err := strconv.Atoi(header); err == nil {
				retryAfter = &seconds
			}
		}
		return &Error{Status: resp.StatusCode, Code: data.Error.Code, Index: data.Error.Index, TokenCount: data.Error.TokenCount, RetryAfter: retryAfter}
	}
	dec.DisallowUnknownFields()
	if err := dec.Decode(out); err != nil {
		return fmt.Errorf("decode embedder response: %w", err)
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return fmt.Errorf("invalid trailing response JSON")
	}
	return nil
}

func (c *Client) Recipe(ctx context.Context) (Report, error) {
	var r Report
	if err := c.request(ctx, "/v1/recipe", nil, &r); err != nil {
		return Report{}, err
	}
	if r.ProtocolVersion != 1 || r.RecipeHash != r.Recipe.Hash() {
		return Report{}, fmt.Errorf("invalid recipe report")
	}
	return r, nil
}

type textRequest struct {
	RecipeHash string   `json:"recipe_hash"`
	Texts      []string `json:"texts"`
}

func (c *Client) Tokenize(ctx context.Context, hash string, texts []string) ([]int, error) {
	var r struct {
		RecipeHash string `json:"recipe_hash"`
		Counts     []int  `json:"counts"`
	}
	if err := c.request(ctx, "/v1/tokenize", textRequest{hash, texts}, &r); err != nil {
		return nil, err
	}
	if r.RecipeHash != hash || len(r.Counts) != len(texts) {
		return nil, fmt.Errorf("invalid tokenize response")
	}
	for _, n := range r.Counts {
		if n <= 0 {
			return nil, fmt.Errorf("invalid token count")
		}
	}
	return r.Counts, nil
}

func (c *Client) Embed(ctx context.Context, hash string, texts []string) ([]Vector, error) {
	var r struct {
		RecipeHash string `json:"recipe_hash"`
		Dimensions int    `json:"dimensions"`
		Data       []struct {
			Index      int       `json:"index"`
			TokenCount int       `json:"token_count"`
			Embedding  []float32 `json:"embedding"`
		} `json:"data"`
	}
	if err := c.request(ctx, "/v1/embed", textRequest{hash, texts}, &r); err != nil {
		return nil, err
	}
	if r.RecipeHash != hash || r.Dimensions <= 0 || len(r.Data) != len(texts) {
		return nil, fmt.Errorf("invalid embed response")
	}
	out := make([]Vector, len(r.Data))
	for i, v := range r.Data {
		if v.Index != i || v.TokenCount <= 0 || len(v.Embedding) != r.Dimensions {
			return nil, fmt.Errorf("invalid embed row %d", i)
		}
		out[i] = Vector{Values: v.Embedding, TokenCount: v.TokenCount, RecipeHash: r.RecipeHash}
	}
	return out, nil
}

type Query struct {
	Client         *Client
	RecipeHash     string
	ExpectedRecipe *Recipe
}

func (q Query) Recipe(ctx context.Context) (Recipe, error) {
	if q.ExpectedRecipe != nil && q.ExpectedRecipe.Hash() != q.RecipeHash {
		return Recipe{}, fmt.Errorf("query recipe mismatch: expected recipe hash %q, configured hash %q", q.ExpectedRecipe.Hash(), q.RecipeHash)
	}
	r, err := q.Client.Recipe(ctx)
	if err != nil {
		return Recipe{}, err
	}
	if r.RecipeHash != q.RecipeHash {
		if q.ExpectedRecipe != nil {
			return Recipe{}, fmt.Errorf("query recipe mismatch: %s", recipeDifferences(*q.ExpectedRecipe, r.Recipe))
		}
		return Recipe{}, fmt.Errorf("query recipe mismatch: expected hash %q, running hash %q", q.RecipeHash, r.RecipeHash)
	}
	return r.Recipe, nil
}

func (q Query) Embed(ctx context.Context, text string) (Vector, error) {
	v, err := q.Client.Embed(ctx, q.RecipeHash, []string{text})
	if err != nil {
		return Vector{}, err
	}
	return v[0], nil
}
