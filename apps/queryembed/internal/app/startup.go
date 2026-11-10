package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"path/filepath"
	"strings"

	"danny.vn/trachuan/apps/queryembed/internal/bundle"
	"danny.vn/trachuan/apps/queryembed/internal/httpapi"
	"danny.vn/trachuan/apps/queryembed/internal/onnx"
)

func startup(ctx context.Context, dir, lib string, n Natives, stderr io.Writer) (*httpapi.Server, func(), error) {
	m, err := bundle.Load(dir)
	if err != nil {
		return nil, nil, fmt.Errorf("load bundle: %w", err)
	}
	if _, err = bundle.QueryRecipe(m, bundle.Versions{}); err != nil {
		return nil, nil, fmt.Errorf("check recipe: %w", err)
	}
	paths := []string{bundle.QueryModelPath}
	foundTokenizer := false
	for _, f := range bundle.TokenizerFiles(m) {
		paths = append(paths, f.Path)
		foundTokenizer = foundTokenizer || f.Path == "tokenizer.json"
	}
	if !foundTokenizer {
		return nil, nil, errors.New("tokenizer file missing")
	}
	if err = m.Verify(dir, paths); err != nil {
		return nil, nil, fmt.Errorf("verify bundle: %w", err)
	}
	tok, tc, err := n.OpenTokenizer(filepath.Join(dir, "tokenizer.json"))
	if err != nil {
		return nil, nil, fmt.Errorf("open tokenizer: %w", err)
	}
	closeTok := func() { _ = tc.Close() }
	ids, err := tok.Encode(strings.Repeat("xin chào ", 300))
	if err != nil || len(ids) <= 256 {
		closeTok()
		return nil, nil, errors.New("tokenizer probe failed")
	}
	model, dimensions, mc, err := n.OpenModel(filepath.Join(dir, bundle.QueryModelPath), onnx.QueryOptions(lib))
	if err != nil {
		closeTok()
		return nil, nil, fmt.Errorf("open model: %w", err)
	}
	closeAll := func() { _ = mc.Close(); closeTok() }
	if dimensions != 1024 {
		closeAll()
		return nil, nil, errors.New("invalid dimensions")
	}
	ids, err = tok.Encode("khởi động")
	if err != nil {
		closeAll()
		return nil, nil, fmt.Errorf("encode warmup: %w", err)
	}
	v, err := model.Embed(ctx, ids)
	if err != nil || !validVector(v, dimensions) {
		closeAll()
		return nil, nil, errors.New("warmup failed")
	}
	versions, err := n.Versions()
	if err != nil {
		closeAll()
		return nil, nil, fmt.Errorf("read versions: %w", err)
	}
	recipe, err := bundle.QueryRecipe(m, versions)
	if err != nil {
		closeAll()
		return nil, nil, fmt.Errorf("build recipe: %w", err)
	}
	return newService(tok, model, recipe, stderr), closeAll, nil
}
func validVector(v []float32, dimensions int) bool {
	if len(v) != dimensions {
		return false
	}
	var sum float64
	for _, x := range v {
		f := float64(x)
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return false
		}
		sum += f * f
	}
	return math.Abs(math.Sqrt(sum)-1) <= 0.001
}
