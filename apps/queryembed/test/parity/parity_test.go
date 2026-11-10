//go:build ort && parity

package parity

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
	"time"

	"danny.vn/trachuan/apps/queryembed/internal/tokenizer"
)

type service struct {
	url, hash string
	client    *http.Client
}

type result struct {
	Counts []int `json:"counts"`
	Data   []struct {
		TokenCount int       `json:"token_count"`
		Embedding  []float64 `json:"embedding"`
	} `json:"data"`
	Error struct {
		TokenCount int `json:"token_count"`
	} `json:"error"`
}

func TestParity_GoMatchesPython(t *testing.T) {
	goURL, pyURL := os.Getenv("QUERYEMBED_PARITY_GO"), os.Getenv("QUERYEMBED_PARITY_PY")
	idsPath, modelDir := os.Getenv("QUERYEMBED_PARITY_PY_IDS"), os.Getenv("QUERYEMBED_PARITY_MODEL_DIR")
	if goURL == "" || pyURL == "" || idsPath == "" || modelDir == "" {
		t.Skip("parity environment is not configured")
	}
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "../../../.."))
	cases := os.Getenv("QUERYEMBED_PARITY_CASES")
	if cases == "" {
		cases = filepath.Join(root, "apps/corpus/internal/evaluation/testdata/vn/legacy.json")
	}
	var legacy []struct {
		Question string `json:"question"`
	}
	readJSON(t, cases, &legacy)
	texts := make([]string, 0, len(legacy))
	for _, c := range legacy {
		texts = append(texts, c.Question)
	}
	var edges []string
	readJSON(t, filepath.Join(filepath.Dir(file), "edge_texts.json"), &edges)
	if len(edges) < 2 {
		t.Fatal("missing boundary texts")
	}
	texts = append(texts, edges...)
	var python struct {
		IDs [][]int64 `json:"ids"`
	}
	readJSON(t, idsPath, &python)
	if len(python.IDs) != len(texts) {
		t.Fatalf("input count: Go %d Python %d", len(texts), len(python.IDs))
	}
	tok, err := tokenizer.Open(filepath.Join(modelDir, "tokenizer.json"))
	if err != nil {
		t.Fatal("cannot open tokenizer")
	}
	defer func() {
		if tok.Close() != nil {
			t.Error("cannot close tokenizer")
		}
	}()
	goService, pyService := openService(t, goURL), openService(t, pyURL)
	minimum, compared := 1.0, 0
	for i, text := range texts {
		ids, err := tok.Encode(text)
		if err != nil {
			t.Errorf("text %d: tokenizer error", i)
			continue
		}
		if !slices.Equal(ids, python.IDs[i]) {
			t.Errorf("text %d: ids differ, Go %d Python %d", i, len(ids), len(python.IDs[i]))
		}
		if i >= len(texts)-2 {
			want := 256 + i - (len(texts) - 2)
			if len(ids) != want {
				t.Errorf("text %d: boundary count, Go %d expected %d", i, len(ids), want)
			}
		}
		gs, gr, ge := goService.post("/v1/tokenize", text)
		ps, pr, pe := pyService.post("/v1/tokenize", text)
		if ge != nil || pe != nil || gs != 200 || ps != 200 || len(gr.Counts) != 1 || len(pr.Counts) != 1 {
			t.Errorf("text %d: tokenize status, Go %d Python %d", i, gs, ps)
		} else if gr.Counts[0] != pr.Counts[0] {
			t.Errorf("text %d: tokenize count, Go %d Python %d", i, gr.Counts[0], pr.Counts[0])
		}
		gs, gr, ge = goService.post("/v1/embed", text)
		ps, pr, pe = pyService.post("/v1/embed", text)
		if ge != nil || pe != nil || (gs != 200 && gs != 422) || (ps != 200 && ps != 422) {
			t.Errorf("text %d: embed status, Go %d Python %d", i, gs, ps)
			continue
		}
		if (gs == 422) != (ps == 422) {
			t.Errorf("text %d: rejection decision, Go %d Python %d", i, gs, ps)
		}
		if i == len(texts)-2 && (gs != 200 || ps != 200) {
			t.Errorf("text %d: boundary acceptance, Go %d Python %d", i, gs, ps)
		}
		if i == len(texts)-1 && (gs != 422 || ps != 422 || gr.Error.TokenCount != 257 || pr.Error.TokenCount != 257) {
			t.Errorf("text %d: boundary rejection counts, Go %d Python %d", i, gr.Error.TokenCount, pr.Error.TokenCount)
		}
		if gs != 200 || ps != 200 {
			continue
		}
		if len(gr.Data) != 1 || len(pr.Data) != 1 {
			t.Errorf("text %d: vector count, Go %d Python %d", i, len(gr.Data), len(pr.Data))
			continue
		}
		cos, ok := cosine(gr.Data[0].Embedding, pr.Data[0].Embedding)
		if !ok {
			t.Errorf("text %d: vector dimensions, Go %d Python %d", i, len(gr.Data[0].Embedding), len(pr.Data[0].Embedding))
			continue
		}
		compared++
		minimum = math.Min(minimum, cos)
		if cos < 0.9999 {
			t.Errorf("text %d: cosine below threshold, Go %d Python %d", i, gr.Data[0].TokenCount, pr.Data[0].TokenCount)
		}
	}
	if compared > 0 {
		t.Logf("minimum cosine: %.8f", minimum)
	}
}

func readJSON(t *testing.T, path string, dst any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil || json.Unmarshal(data, dst) != nil {
		t.Fatal("cannot read parity input")
	}
}

func openService(t *testing.T, raw string) service {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "http" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" {
		t.Fatal("invalid parity service URL")
	}
	ip := net.ParseIP(u.Hostname())
	if u.Hostname() != "localhost" && (ip == nil || !ip.IsLoopback()) {
		t.Fatal("parity service must use loopback")
	}
	client := &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }, Transport: &http.Transport{Proxy: nil}}
	resp, err := client.Get(raw + "/v1/recipe")
	if err != nil {
		t.Fatal("cannot reach parity service")
	}
	defer func() { _ = resp.Body.Close() }()
	var recipe struct {
		Hash string `json:"recipe_hash"`
	}
	if resp.StatusCode != 200 || json.NewDecoder(resp.Body).Decode(&recipe) != nil || recipe.Hash == "" {
		t.Fatal("cannot read parity recipe")
	}
	return service{raw, recipe.Hash, client}
}

func (s service) post(path, text string) (int, result, error) {
	body, err := json.Marshal(struct {
		Hash  string   `json:"recipe_hash"`
		Texts []string `json:"texts"`
	}{s.hash, []string{text}})
	if err != nil {
		return 0, result{}, err
	}
	resp, err := s.client.Post(s.url+path, "application/json", bytes.NewReader(body))
	if err != nil {
		return 0, result{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	var out result
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return resp.StatusCode, out, fmt.Errorf("decode response: %w", err)
	}
	return resp.StatusCode, out, nil
}

func cosine(a, b []float64) (float64, bool) {
	if len(a) == 0 || len(a) != len(b) {
		return 0, false
	}
	var dot, aa, bb float64
	for i, v := range a {
		dot += v * b[i]
		aa += v * v
		bb += b[i] * b[i]
	}
	value := dot / math.Sqrt(aa*bb)
	return value, !math.IsNaN(value) && !math.IsInf(value, 0)
}
