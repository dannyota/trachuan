package fakeembed

import (
	"context"
	"io"
	"log/slog"
	"net/http"

	"danny.vn/trachuan/apps/server/cmd/fakeembed/internal/admission"
	"danny.vn/trachuan/internal/embedding"
)

type fakeRuntime struct{}

func (fakeRuntime) Encode(text string) ([]int64, error) {
	ids := make([]int64, 0, len(text)+2)
	ids = append(ids, -1)
	for _, r := range text {
		ids = append(ids, int64(r))
	}
	return append(ids, -1), nil
}

func (fakeRuntime) Embed(_ context.Context, ids []int64) ([]float32, error) {
	runes := make([]rune, 0, len(ids)-2)
	for _, id := range ids[1 : len(ids)-1] {
		runes = append(runes, rune(id))
	}
	return embedding.FakeVector(string(runes)), nil
}

// NewHandler returns the ready test embedding service.
func NewHandler() http.Handler {
	runtime := fakeRuntime{}
	s := New(Config{Tokenizer: runtime, Runtime: runtime, Recipe: embedding.TestRecipe(), Admission: admission.New(admission.QueryLimits()), Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	s.SetReady(true)
	return s.Handler()
}
