//go:build ort

package main

import (
	"context"
	"io"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"

	"danny.vn/trachuan/apps/queryembed/internal/app"
	"danny.vn/trachuan/apps/queryembed/internal/bundle"
	"danny.vn/trachuan/apps/queryembed/internal/httpapi"
	"danny.vn/trachuan/apps/queryembed/internal/onnx"
	"danny.vn/trachuan/apps/queryembed/internal/tokenizer"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	n := app.Natives{
		OpenTokenizer: func(path string) (httpapi.Tokenizer, io.Closer, error) {
			t, err := tokenizer.Open(path)
			if err != nil {
				return nil, nil, err
			}
			return t, t, nil
		},
		OpenModel: func(path string, o onnx.Options) (httpapi.Runtime, int, io.Closer, error) {
			m, err := onnx.Open(path, o)
			if err != nil {
				return nil, 0, nil, err
			}
			return m, m.Dimensions(), m, nil
		},
		Versions: func() (bundle.Versions, error) {
			v, err := onnx.Version()
			return bundle.Versions{Go: strings.TrimPrefix(runtime.Version(), "go"), Tokenizers: tokenizer.Version(), ONNXRuntime: v}, err
		},
	}
	os.Exit(app.Run(ctx, os.Args[1:], os.Stderr, n))
}
