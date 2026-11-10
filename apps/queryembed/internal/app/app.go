// Package app starts and stops the query embedding service.
package app

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"time"

	"danny.vn/trachuan/apps/queryembed/internal/admission"
	"danny.vn/trachuan/apps/queryembed/internal/bundle"
	"danny.vn/trachuan/apps/queryembed/internal/httpapi"
	"danny.vn/trachuan/apps/queryembed/internal/onnx"
	"danny.vn/trachuan/internal/embedding"
)

// Natives supplies the native operations used at startup.
type Natives struct {
	OpenTokenizer func(string) (httpapi.Tokenizer, io.Closer, error)
	OpenModel     func([]byte, onnx.Options) (httpapi.Runtime, int, io.Closer, error)
	Versions      func() (bundle.Versions, error)
}

// Run starts the service and returns its process exit code.
func Run(ctx context.Context, args []string, stderr io.Writer, n Natives) int {
	return run(ctx, args, stderr, n, net.Listen, nil)
}

func run(ctx context.Context, args []string, stderr io.Writer, n Natives, listenFn func(string, string) (net.Listener, error), unready func()) int {
	flags := flag.NewFlagSet("queryembed", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	dir := flags.String("model-dir", "", "model bundle")
	lib := flags.String("ort-lib", "", "ONNX Runtime library")
	listen := flags.String("listen", "127.0.0.1:8091", "listen address")
	if flags.Parse(args) != nil || flags.NArg() != 0 || *dir == "" || *lib == "" || n.OpenTokenizer == nil || n.OpenModel == nil || n.Versions == nil {
		_, _ = fmt.Fprintln(stderr, "queryembed: invalid startup configuration (--model-dir, --ort-lib, --listen)")
		return 1
	}
	service, closeNatives, err := startup(ctx, *dir, *lib, n, stderr)
	if err != nil {
		if ctx.Err() != nil {
			return 0
		}
		_, _ = fmt.Fprintln(stderr, "queryembed: startup failed")
		return 1
	}
	defer closeNatives()
	if ctx.Err() != nil {
		return 0
	}
	listener, err := listenFn("tcp", *listen)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "queryembed: startup failed")
		return 1
	}
	server := &http.Server{Handler: service.Handler(), ReadHeaderTimeout: 5 * time.Second}
	service.SetReady(true)
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	select {
	case <-ctx.Done():
		service.SetReady(false)
		if unready != nil {
			unready()
		}
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			_ = server.Close()
		}
		if serveErr := <-done; serveErr != nil && serveErr != http.ErrServerClosed {
			_, _ = fmt.Fprintln(stderr, "queryembed: server failed")
		}
		return 0
	case err := <-done:
		service.SetReady(false)
		if err != nil && err != http.ErrServerClosed {
			_, _ = fmt.Fprintln(stderr, "queryembed: server failed")
			return 1
		}
		return 0
	}
}

func newService(tok httpapi.Tokenizer, model httpapi.Runtime, recipe embedding.Recipe, stderr io.Writer) *httpapi.Server {
	return httpapi.New(httpapi.Config{Tokenizer: tok, Runtime: model, Recipe: recipe, Admission: admission.New(admission.QueryLimits()), Logger: slog.New(slog.NewTextHandler(stderr, nil))})
}
