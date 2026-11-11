package retrieval

import (
	"context"
	"errors"
	"math"
	"time"
	"unicode/utf8"

	"danny.vn/trachuan/internal/embedding"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Engine holds one corpus and its query runtime.
type Engine struct {
	db          *pgxpool.Pool
	release     Release
	options     Options
	embedder    Embedder
	queryRecipe embedding.Recipe
}

// Prepare validates and embeds an accepted query exactly once.
func (e *Engine) Prepare(ctx context.Context, req Request) (Prepared, error) {
	return e.prepare(ctx, req, false)
}

// PrepareLegacy skips the diacritics gate for trusted evaluation.
func (e *Engine) PrepareLegacy(ctx context.Context, req Request) (Prepared, error) {
	return e.prepare(ctx, req, true)
}
func (e *Engine) prepare(ctx context.Context, req Request, legacy bool) (Prepared, error) {
	if req.Limit < 0 || req.Limit > 50 || (req.Mode != "" && req.Mode != "ann" && req.Mode != "exact") || (req.Arms != "" && req.Arms != "hybrid" && req.Arms != "dense" && req.Arms != "lexical") || utf8.RuneCountInString(req.Query) > 2000 {
		return Prepared{}, errors.New("invalid_request")
	}
	if req.AsOf != nil {
		d := req.AsOf
		t := time.Date(d.Year, d.Month, d.Day, 0, 0, 0, 0, time.UTC)
		if d.Year < 1 || d.Year > 9999 || t.Year() != d.Year || t.Month() != d.Month || t.Day() != d.Day {
			return Prepared{}, errors.New("invalid_request")
		}
	}
	v, err := ValidateQuery(req.Query)
	if err != nil {
		return Prepared{}, err
	}
	if legacy && v.Reason == "diacritics_required" {
		v.Accepted = true
		v.Reason = ""
		v.Message = ""
	}
	p := Prepared{Request: req, Validation: v}
	if !v.Accepted {
		return p, nil
	}
	vector, err := e.embedder.Embed(ctx, req.Query)
	if err != nil {
		return Prepared{}, embedderFailure("query embedding failed", err)
	}
	if vector.RecipeHash != e.queryRecipe.Hash() || len(vector.Values) != e.queryRecipe.Dimensions || vector.TokenCount <= 0 || vector.TokenCount > e.queryRecipe.TokenCap {
		return Prepared{}, classifiedFailure{message: "invalid query embedding", kind: ErrEmbedder}
	}
	var n float64
	for _, x := range vector.Values {
		f := float64(x)
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return Prepared{}, classifiedFailure{message: "invalid query embedding", kind: ErrEmbedder}
		}
		n += f * f
	}
	if math.Abs(math.Sqrt(n)-1) > .001 {
		return Prepared{}, classifiedFailure{message: "invalid query embedding", kind: ErrEmbedder}
	}
	p.Vector = &vector
	return p, nil
}
