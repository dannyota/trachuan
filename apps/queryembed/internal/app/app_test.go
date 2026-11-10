package app

import (
	"context"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/apps/queryembed/internal/bundle"
	"danny.vn/trachuan/apps/queryembed/internal/httpapi"
	"danny.vn/trachuan/apps/queryembed/internal/onnx"
)

type fakeTokenizer struct{ truncate bool }

func (f fakeTokenizer) Encode(s string) ([]int64, error) {
	if f.truncate {
		return make([]int64, 256), nil
	}
	return make([]int64, len(strings.Fields(s))+2), nil
}

type fakeModel struct {
	warm    chan struct{}
	release chan struct{}
}

func (f fakeModel) Embed(ctx context.Context, _ []int64) ([]float32, error) {
	if f.warm != nil {
		close(f.warm)
		<-f.release
	}
	v := make([]float32, 1024)
	v[0] = 1
	return v, nil
}

type closer struct{}

func (closer) Close() error { return nil }
func natives(truncate bool, m fakeModel) Natives {
	return Natives{
		OpenTokenizer: func(string) (httpapi.Tokenizer, io.Closer, error) { return fakeTokenizer{truncate}, closer{}, nil },
		OpenModel:     func(string, onnx.Options) (httpapi.Runtime, int, io.Closer, error) { return m, 1024, closer{}, nil },
		Versions: func() (bundle.Versions, error) {
			return bundle.Versions{Go: "1", Tokenizers: "1", ONNXRuntime: "1"}, nil
		},
	}
}
func TestRun_RefusesTruncatingTokenizer(t *testing.T) {
	ctx := context.Background()
	if got := Run(ctx, []string{"--model-dir", filepath.Join("..", "bundle", "testdata"), "--ort-lib", "test"}, io.Discard, natives(true, fakeModel{})); got != 1 {
		t.Fatalf("got %d", got)
	}
}
func TestRun_ReadyOnlyAfterWarmup(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	_ = l.Close()
	warm, release := make(chan struct{}), make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan int, 1)
	go func() {
		done <- Run(ctx, []string{"--model-dir", filepath.Join("..", "bundle", "testdata"), "--ort-lib", "test", "--listen", addr}, io.Discard, natives(false, fakeModel{warm, release}))
	}()
	<-warm
	client := http.Client{Timeout: 50 * time.Millisecond}
	if resp, e := client.Get("http://" + addr + "/readyz"); e == nil {
		_ = resp.Body.Close()
		if resp.StatusCode == 200 {
			t.Fatal("ready before warmup")
		}
	}
	close(release)
	cancel()
	if got := <-done; got != 0 {
		t.Fatalf("got %d", got)
	}
}

func TestStartup_OrderAndBundleMismatch(t *testing.T) {
	dir := filepath.Join("..", "bundle", "testdata")
	calls := []string{}
	n := natives(false, fakeModel{})
	openTokenizer := n.OpenTokenizer
	n.OpenTokenizer = func(path string) (httpapi.Tokenizer, io.Closer, error) {
		calls = append(calls, "tokenizer")
		return openTokenizer(path)
	}
	openModel := n.OpenModel
	n.OpenModel = func(path string, options onnx.Options) (httpapi.Runtime, int, io.Closer, error) {
		calls = append(calls, "model")
		return openModel(path, options)
	}
	service, closeAll, err := startup(context.Background(), dir, "library", n, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	closeAll()
	if service == nil || strings.Join(calls, ",") != "tokenizer,model" {
		t.Fatalf("startup order: %v", calls)
	}
	calls = nil
	_, _, err = startup(context.Background(), t.TempDir(), "library", n, io.Discard)
	if err == nil || len(calls) != 0 {
		t.Fatalf("invalid bundle: err=%v calls=%v", err, calls)
	}
}

func TestRun_CancelDuringWarmup(t *testing.T) {
	warm, release := make(chan struct{}), make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	n := natives(false, fakeModel{warm, release})
	n.Versions = func() (bundle.Versions, error) { return bundle.Versions{}, context.Canceled }
	done := make(chan int, 1)
	go func() {
		done <- Run(ctx, []string{"--model-dir", filepath.Join("..", "bundle", "testdata"), "--ort-lib", "test"}, io.Discard, n)
	}()
	<-warm
	cancel()
	close(release)
	if got := <-done; got != 0 {
		t.Fatalf("got %d", got)
	}
}
