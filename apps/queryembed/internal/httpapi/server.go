// Package httpapi serves the query embedding protocol.
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"sync/atomic"
	"time"

	"danny.vn/trachuan/apps/queryembed/internal/admission"
	"danny.vn/trachuan/internal/embedding"
)

// Tokenizer encodes text with special tokens and without truncation.
type Tokenizer interface {
	Encode(text string) ([]int64, error)
}

// Runtime embeds and normalizes a single token sequence.
type Runtime interface {
	Embed(ctx context.Context, ids []int64) ([]float32, error)
}

// Config supplies the service dependencies.
type Config struct {
	Tokenizer Tokenizer
	Runtime   Runtime
	Recipe    embedding.Recipe
	Admission *admission.Admission
	Logger    *slog.Logger
}

// Server serves the query embedding protocol.
type Server struct {
	cfg   Config
	hash  string
	ready atomic.Bool
}

// New constructs a server that starts unready.
func New(cfg Config) *Server { return &Server{cfg: cfg, hash: cfg.Recipe.Hash()} }

// SetReady changes whether inference and recipe reporting are available.
func (s *Server) SetReady(ready bool) { s.ready.Store(ready) }

// Handler returns the HTTP handler.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.health)
	mux.HandleFunc("GET /readyz", s.readiness)
	mux.HandleFunc("GET /v1/recipe", s.recipe)
	mux.HandleFunc("POST /v1/tokenize", func(w http.ResponseWriter, r *http.Request) { s.post(w, r, false) })
	mux.HandleFunc("POST /v1/embed", func(w http.ResponseWriter, r *http.Request) { s.post(w, r, true) })
	return mux
}

type response struct {
	status      int
	code        string
	payload     []byte
	count       int
	bodyTimeout bool
}

func jsonResponse(v any) response {
	b, err := json.Marshal(v)
	if err != nil {
		return failure(500, "inference_failed", nil)
	}
	return response{status: 200, payload: b}
}
func (s *Server) serve(w http.ResponseWriter, r *http.Request, fn func() response) {
	start := time.Now()
	res := fn()
	if r.Context().Err() == nil || res.bodyTimeout {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(res.status)
		_, _ = w.Write(res.payload)
	}
	s.cfg.Logger.Info("request", "route", r.URL.Path, "status", res.status, "code", res.code, "token_count", res.count, "latency", time.Since(start))
}
func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	s.serve(w, r, func() response {
		return jsonResponse(struct {
			Status string `json:"status"`
		}{"alive"})
	})
}
func (s *Server) readiness(w http.ResponseWriter, r *http.Request) {
	s.serve(w, r, func() response {
		if !s.ready.Load() {
			return failure(503, "not_ready", nil)
		}
		return jsonResponse(struct {
			Status string `json:"status"`
			Hash   string `json:"recipe_hash"`
		}{"ready", s.hash})
	})
}
func (s *Server) recipe(w http.ResponseWriter, r *http.Request) {
	s.serve(w, r, func() response {
		if !s.ready.Load() {
			return failure(503, "not_ready", nil)
		}
		return jsonResponse(struct {
			Protocol int              `json:"protocol_version"`
			Hash     string           `json:"recipe_hash"`
			Recipe   embedding.Recipe `json:"recipe"`
		}{1, s.hash, s.cfg.Recipe})
	})
}
func (s *Server) post(w http.ResponseWriter, r *http.Request, embed bool) {
	s.serve(w, r, func() response { return s.process(w, r, embed) })
}
func (s *Server) process(w http.ResponseWriter, r *http.Request, embed bool) response {
	ctx, cancel := context.WithTimeout(r.Context(), admission.QueryLimits().Deadline)
	defer cancel()
	if deadline, ok := ctx.Deadline(); ok {
		controller := http.NewResponseController(w)
		_ = controller.SetReadDeadline(deadline)
	}
	if !s.ready.Load() {
		w.Header().Set("Connection", "close")
		return failure(503, "not_ready", nil)
	}
	text, problem := decodeRequest(r, s.hash)
	if problem.status != 0 {
		w.Header().Set("Connection", "close")
		return problem
	}
	_ = http.NewResponseController(w).SetReadDeadline(time.Time{})
	if ctx.Err() != nil {
		return failure(504, "deadline_exceeded", nil)
	}
	var result response
	err := s.cfg.Admission.Do(ctx, func(workCtx context.Context) error {
		ids, e := s.cfg.Tokenizer.Encode(text)
		if err := workCtx.Err(); err != nil {
			return err
		}
		if e != nil {
			result = failure(500, "inference_failed", nil)
			return nil
		}
		count := len(ids)
		if !embed {
			result = jsonResponse(struct {
				Hash   string `json:"recipe_hash"`
				Counts []int  `json:"counts"`
			}{s.hash, []int{count}})
			result.count = count
			return nil
		}
		if count > s.cfg.Recipe.TokenCap {
			result = failure(422, "input_too_long", &count)
			result.count = count
			return nil
		}
		vector, e := s.cfg.Runtime.Embed(workCtx, ids)
		if err := workCtx.Err(); err != nil {
			return err
		}
		if e != nil || !validVector(vector, s.cfg.Recipe.Dimensions) {
			result = failure(500, "inference_failed", nil)
			result.count = count
			return nil
		}
		type item struct {
			Index      int       `json:"index"`
			TokenCount int       `json:"token_count"`
			Embedding  []float32 `json:"embedding"`
		}
		result = jsonResponse(struct {
			Hash       string `json:"recipe_hash"`
			Dimensions int    `json:"dimensions"`
			Data       []item `json:"data"`
		}{s.hash, s.cfg.Recipe.Dimensions, []item{{0, count, vector}}})
		result.count = count
		return nil
	})
	if errors.Is(err, admission.ErrBusy) {
		return failure(429, "busy", nil)
	}
	if errors.Is(err, admission.ErrDeadline) || errors.Is(err, context.DeadlineExceeded) {
		return failure(504, "deadline_exceeded", nil)
	}
	if err != nil {
		return failure(500, "inference_failed", nil)
	}
	return result
}
