package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"reflect"
	"strings"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/stage/chunk"
	lawembed "danny.vn/trachuan/apps/corpus/internal/stage/embed"
	"danny.vn/trachuan/internal/embedding"
	"danny.vn/trachuan/internal/frameworks/build"
)

type frameworkEmbedError struct{ cause error }

func (e *frameworkEmbedError) Error() string { return "operation failed" }
func (e *frameworkEmbedError) Unwrap() error { return e.cause }

type frameworkEmbedder struct {
	client    *embedding.Client
	expected  embedding.Recipe
	configErr error
}

func newFrameworkEmbedder() *frameworkEmbedder {
	address := os.Getenv("CORPUS_EMBEDDER_URL")
	if address == "" {
		address = "http://127.0.0.1:8092"
	}
	u, err := url.Parse(address)
	if err != nil || u.Scheme != "http" || u.User != nil || u.Host == "" || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return &frameworkEmbedder{configErr: fmt.Errorf("invalid local embedder URL")}
	}
	host := u.Hostname()
	ip, ipErr := netip.ParseAddr(host)
	if host != "localhost" && (ipErr != nil || !ip.IsLoopback()) {
		return &frameworkEmbedder{configErr: fmt.Errorf("embedder URL must use loopback")}
	}
	port := u.Port()
	if port == "" {
		port = "80"
	}
	if host == "localhost" {
		host = "127.0.0.1"
	}
	target := net.JoinHostPort(host, port)
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "tcp", target)
	}}
	hc := &http.Client{Transport: transport, Timeout: 125 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return &frameworkEmbedder{client: embedding.NewClient(address, hc), expected: embedding.DocumentRecipe1()}
}

func (s *frameworkEmbedder) Check(ctx context.Context) error {
	if s.configErr != nil {
		return s.configErr
	}
	report, err := s.client.Recipe(ctx)
	if err != nil {
		return fmt.Errorf("read embedder recipe: %w", &frameworkEmbedError{cause: err})
	}
	if report.Recipe != s.expected || report.RecipeHash != s.expected.Hash() {
		want, got := reflect.ValueOf(s.expected), reflect.ValueOf(report.Recipe)
		var fields []string
		for i := 0; i < want.NumField(); i++ {
			if want.Field(i).Interface() != got.Field(i).Interface() {
				fields = append(fields, want.Type().Field(i).Tag.Get("json"))
			}
		}
		if len(fields) == 0 {
			fields = append(fields, "recipe_hash")
		}
		return fmt.Errorf("embedder recipe mismatch: %s", strings.Join(fields, ", "))
	}
	return nil
}

func (s *frameworkEmbedder) Build(ctx context.Context, key string, rows []build.Chunk, hash string) (build.EmbedResult, error) {
	file := chunk.File{Version: 1, DocumentKey: key}
	for _, row := range rows {
		file.Chunks = append(file.Chunks, chunk.Chunk{Ordinal: row.Ordinal, ContextPrefix: row.ContextPrefix, Content: row.Content})
	}
	idx, vectors, err := lawembed.BuildUncached(ctx, s.client, s.expected, file, hash)
	if err != nil {
		return build.EmbedResult{}, fmt.Errorf("embedder request: %w", &frameworkEmbedError{cause: err})
	}
	data, err := json.Marshal(idx)
	if err != nil {
		return build.EmbedResult{}, fmt.Errorf("encode index: %w", err)
	}
	result := build.EmbedResult{Index: data, Vectors: vectors, Rows: len(idx.Rows)}
	for _, reject := range idx.Rejected {
		result.Rejected = append(result.Rejected, build.EmbedRejection{Ordinal: reject.ChunkOrdinal, TokenCount: reject.TokenCount, Kind: reject.Kind})
	}
	return result, nil
}
