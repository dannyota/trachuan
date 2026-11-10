package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"danny.vn/trachuan/apps/queryembed/internal/admission"
	"danny.vn/trachuan/internal/embedding"
)

type fakeTokenizer struct{ fail bool }

func (f fakeTokenizer) Encode(s string) ([]int64, error) {
	if f.fail {
		return nil, errors.New("secret")
	}
	return make([]int64, len(strings.Fields(s))+2), nil
}

type fakeRuntime struct {
	vector []float32
	calls  atomic.Int32
	wait   bool
}

func (f *fakeRuntime) Embed(ctx context.Context, _ []int64) ([]float32, error) {
	f.calls.Add(1)
	if f.wait {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return f.vector, nil
}
func setup(t *testing.T) (*Server, *fakeRuntime, embedding.Recipe, *bytes.Buffer) {
	t.Helper()
	r := embedding.Recipe{Dimensions: 2, TokenCap: 256}
	rt := &fakeRuntime{vector: []float32{0.6, 0.8}}
	logs := new(bytes.Buffer)
	s := New(Config{Tokenizer: fakeTokenizer{}, Runtime: rt, Recipe: r, Admission: admission.New(admission.QueryLimits()), Logger: slog.New(slog.NewTextHandler(logs, nil))})
	return s, rt, r, logs
}
func request(s *Server, method, path, body, media string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if media != "" {
		req.Header.Set("Content-Type", media)
	}
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	return w
}
func post(s *Server, path, body string) *httptest.ResponseRecorder {
	return request(s, "POST", path, body, "application/json")
}
func body(r embedding.Recipe, text string) string {
	b, _ := json.Marshal(map[string]any{"recipe_hash": r.Hash(), "texts": []string{text}})
	return string(b)
}
func TestHealthz_AliveWhileLoading(t *testing.T) {
	s, _, _, _ := setup(t)
	w := request(s, "GET", "/healthz", "", "")
	if w.Code != 200 || strings.TrimSpace(w.Body.String()) != `{"status":"alive"}` {
		t.Fatal(w.Code, w.Body)
	}
}
func TestReadyzAndRecipe_503UntilReady(t *testing.T) {
	s, _, _, _ := setup(t)
	for _, p := range []string{"/readyz", "/v1/recipe"} {
		if w := request(s, "GET", p, "", ""); w.Code != 503 {
			t.Fatal(p, w.Code)
		}
	}
	s.SetReady(true)
	for _, p := range []string{"/readyz", "/v1/recipe"} {
		if w := request(s, "GET", p, "", ""); w.Code != 200 {
			t.Fatal(p, w.Code)
		}
	}
}
func TestRecipe_ReportShape(t *testing.T) {
	s, _, r, _ := setup(t)
	s.SetReady(true)
	w := request(s, "GET", "/v1/recipe", "", "")
	var got struct {
		Protocol int              `json:"protocol_version"`
		Hash     string           `json:"recipe_hash"`
		Recipe   embedding.Recipe `json:"recipe"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil || got.Protocol != 1 || got.Hash != r.Hash() || got.Recipe != r {
		t.Fatal(w.Body, err)
	}
}
func TestTokenize_CountsWithoutTruncation(t *testing.T) {
	s, _, r, _ := setup(t)
	s.SetReady(true)
	w := post(s, "/v1/tokenize", body(r, strings.Repeat("word ", 300)))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"counts":[302]`) {
		t.Fatal(w.Code, w.Body)
	}
}
func TestEmbed_ReturnsVectorAndTokenCount(t *testing.T) {
	s, _, r, _ := setup(t)
	s.SetReady(true)
	w := post(s, "/v1/embed", body(r, "hello"))
	want := fmt.Sprintf(`{"recipe_hash":%q,"dimensions":2,"data":[{"index":0,"token_count":3,"embedding":[0.6,0.8]}]}`, r.Hash())
	if w.Code != 200 || strings.TrimSpace(w.Body.String()) != want {
		t.Fatal(w.Code, w.Body)
	}
}
func TestEmbed_TokenCap(t *testing.T) {
	s, rt, r, _ := setup(t)
	s.SetReady(true)
	for _, tc := range []struct {
		words, status int
		count         string
	}{{254, 200, ""}, {255, 422, `"index":0,"token_count":257`}} {
		w := post(s, "/v1/embed", body(r, strings.Repeat("a ", tc.words)))
		if w.Code != tc.status || !strings.Contains(w.Body.String(), tc.count) {
			t.Fatal(w.Code, w.Body)
		}
	}
	if rt.calls.Load() != 1 {
		t.Fatal(rt.calls.Load())
	}
}
func TestPost_Validation(t *testing.T) {
	s, _, r, _ := setup(t)
	s.SetReady(true)
	good := body(r, "x")
	tests := []struct {
		name, body, media string
		status            int
	}{
		{"missing", `{}`, "application/json", 400}, {"empty", body(r, " \t"), "application/json", 400}, {"two", fmt.Sprintf(`{"recipe_hash":%q,"texts":["a","b"]}`, r.Hash()), "application/json", 400}, {"unknown", strings.TrimSuffix(good, "}") + `,"extra":1}`, "application/json", 400}, {"duplicate nested", `{"recipe_hash":"x","texts":[{"a":1,"a":2}]}`, "application/json", 400}, {"utf8", string([]byte{'{', '"', 0xff, '"', '}'}), "application/json", 400}, {"oversize", strings.Repeat("x", 65537), "application/json", 413}, {"type", good, "text/plain", 415}, {"mismatch", `{"recipe_hash":"wrong","texts":["x"]}`, "application/json", 409}, {"null text", `{"recipe_hash":"x","texts":[null]}`, "application/json", 400}, {"trailing", good + ` {}`, "application/json", 400}}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := request(s, "POST", "/v1/embed", tc.body, tc.media)
			codes := map[int]string{400: "invalid_request", 409: "recipe_mismatch", 413: "body_too_large", 415: "unsupported_media_type"}
			code := codes[tc.status]
			want := fmt.Sprintf(`{"error":{"code":%q,"message":%q,"index":null,"token_count":null}}`, code, strings.ReplaceAll(code, "_", " "))
			if w.Code != tc.status || w.Body.String() != want {
				t.Fatal(w.Code, w.Body)
			}
		})
	}
	pad := good + strings.Repeat(" ", 65536-len(good))
	if w := post(s, "/v1/embed", pad); w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
}
func TestEmbed_InvalidVector500(t *testing.T) {
	for _, v := range [][]float32{{1}, {float32(math.NaN()), 1}, {1, 1}} {
		s, rt, r, _ := setup(t)
		rt.vector = v
		s.SetReady(true)
		if w := post(s, "/v1/embed", body(r, "x")); w.Code != 500 {
			t.Fatal(w.Code, w.Body)
		}
	}
}
func TestEmbed_BusyAndDeadline(t *testing.T) {
	r := embedding.Recipe{Dimensions: 2, TokenCap: 256}
	for _, tc := range []struct {
		name   string
		limits admission.Limits
		wait   bool
		status int
	}{{"busy", admission.Limits{Active: 0, Queued: 0, Wait: time.Millisecond, Deadline: time.Second}, false, 429}, {"deadline", admission.Limits{Active: 1, Wait: time.Millisecond, Deadline: time.Millisecond}, true, 504}} {
		t.Run(tc.name, func(t *testing.T) {
			s := New(Config{Tokenizer: fakeTokenizer{}, Runtime: &fakeRuntime{wait: tc.wait}, Recipe: r, Admission: admission.New(tc.limits), Logger: slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))})
			s.SetReady(true)
			if w := post(s, "/v1/embed", body(r, "x")); w.Code != tc.status {
				t.Fatal(w.Code, w.Body)
			}
		})
	}
}
func TestErrorsAndLogs_NoInputText(t *testing.T) {
	s, _, r, logs := setup(t)
	s.SetReady(true)
	marker := "private-unique-marker"
	w := post(s, "/v1/embed", body(r, marker))
	if w.Code != http.StatusOK || strings.Contains(logs.String(), marker) || strings.Contains(w.Body.String(), marker) {
		t.Fatal(w.Code, logs.String(), w.Body)
	}
	w = post(s, "/v1/embed", `{"recipe_hash":"`+marker+`","texts":["x"]}`)
	if strings.Contains(w.Body.String(), marker) || strings.Contains(logs.String(), marker) {
		t.Fatal(w.Body, logs.String())
	}
}

type deadlineWriter struct {
	*httptest.ResponseRecorder
	deadline time.Time
	set      bool
}

func (w *deadlineWriter) SetReadDeadline(deadline time.Time) error {
	w.deadline = deadline
	if !deadline.IsZero() {
		w.set = true
	}
	return nil
}

type timeoutBody struct{}

func (timeoutBody) Read([]byte) (int, error) { return 0, timeoutError{} }
func (timeoutBody) Close() error             { return nil }

type timeoutError struct{}

func (timeoutError) Error() string   { return "timeout" }
func (timeoutError) Timeout() bool   { return true }
func (timeoutError) Temporary() bool { return true }
func TestPost_ReadDeadline(t *testing.T) {
	s, _, _, _ := setup(t)
	s.SetReady(true)
	req := httptest.NewRequest("POST", "/v1/embed", nil)
	req.Header.Set("Content-Type", "application/json")
	req.Body = timeoutBody{}
	w := &deadlineWriter{ResponseRecorder: httptest.NewRecorder()}
	s.Handler().ServeHTTP(w, req)
	if !w.set || w.Code != 504 {
		t.Fatal(w.deadline, w.Code, w.Body)
	}
}
func TestPost_DisconnectedClientWritesNothing(t *testing.T) {
	s, _, r, _ := setup(t)
	s.SetReady(true)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req := httptest.NewRequest("POST", "/v1/embed", strings.NewReader(body(r, "x"))).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Body.Len() != 0 || w.Flushed || w.Header().Get("Content-Type") != "" {
		t.Fatal(w.Body, w.Header())
	}
}
