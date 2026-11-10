package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"danny.vn/trachuan/apps/queryembed/internal/bundle"
	"danny.vn/trachuan/apps/queryembed/internal/httpapi"
	"danny.vn/trachuan/apps/queryembed/internal/onnx"
)

func fixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, name := range []string{"bundle.json", "tokenizer.json", bundle.QueryModelPath} {
		data, err := os.ReadFile(filepath.Join("..", "bundle", "testdata", name))
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(dir, name)
		if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}
func TestRun_RefusesBundleMismatch(t *testing.T) {
	for _, kind := range []string{"weights", "model"} {
		t.Run(kind, func(t *testing.T) {
			dir := fixture(t)
			if kind == "weights" {
				if err := os.WriteFile(filepath.Join(dir, bundle.QueryModelPath), []byte("bad"), 0600); err != nil {
					t.Fatal(err)
				}
			} else {
				path := filepath.Join(dir, "bundle.json")
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				var m bundle.Manifest
				if err = json.Unmarshal(data, &m); err != nil {
					t.Fatal(err)
				}
				m.Model = "wrong"
				data, err = json.Marshal(m)
				if err != nil {
					t.Fatal(err)
				}
				if err = os.WriteFile(path, data, 0600); err != nil {
					t.Fatal(err)
				}
			}
			n := natives(false, fakeModel{})
			called := false
			n.OpenTokenizer = func(string) (httpapi.Tokenizer, io.Closer, error) {
				called = true
				return nil, nil, fmt.Errorf("unexpected")
			}
			if code := Run(context.Background(), []string{"--model-dir", dir, "--ort-lib", "lib"}, io.Discard, n); code != 1 || called {
				t.Fatalf("code=%d opened=%v", code, called)
			}
		})
	}
}

type recordingTokenizer struct{ calls *[]string }

func (t recordingTokenizer) Encode(s string) ([]int64, error) {
	if s == strings.Repeat("xin chào ", 300) {
		*t.calls = append(*t.calls, "probe")
		return make([]int64, 300), nil
	}
	if s == "khởi động" {
		*t.calls = append(*t.calls, "warmup tokens")
		return []int64{1}, nil
	}
	return nil, fmt.Errorf("unexpected text")
}

type recordingModel struct{ calls *[]string }

func (m recordingModel) Embed(context.Context, []int64) ([]float32, error) {
	*m.calls = append(*m.calls, "warmup vector")
	v := make([]float32, 1024)
	v[0] = 1
	return v, nil
}
func TestRun_StartupOrder(t *testing.T) {
	dir := fixture(t)
	calls := []string{}
	wantModel, readErr := os.ReadFile(filepath.Join(dir, bundle.QueryModelPath))
	if readErr != nil {
		t.Fatal(readErr)
	}
	n := natives(false, fakeModel{})
	n.OpenTokenizer = func(string) (httpapi.Tokenizer, io.Closer, error) {
		calls = append(calls, "verified and tokenizer")
		if err := os.Remove(filepath.Join(dir, bundle.QueryModelPath)); err != nil {
			return nil, nil, err
		}
		return recordingTokenizer{&calls}, closer{}, nil
	}
	n.OpenModel = func(data []byte, _ onnx.Options) (httpapi.Runtime, int, io.Closer, error) {
		calls = append(calls, "model")
		if !bytes.Equal(data, wantModel) {
			return nil, 0, nil, fmt.Errorf("model bytes differ from verified snapshot")
		}
		return recordingModel{&calls}, 1024, closer{}, nil
	}
	n.Versions = func() (bundle.Versions, error) { calls = append(calls, "versions"); return bundle.Versions{}, nil }
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	address := make(chan string, 1)
	listen := func(_, _ string) (net.Listener, error) {
		calls = append(calls, "listen")
		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err == nil {
			address <- l.Addr().String()
		}
		return l, err
	}
	done := make(chan int, 1)
	go func() { done <- run(ctx, []string{"--model-dir", dir, "--ort-lib", "lib"}, io.Discard, n, listen, nil) }()
	client := &http.Client{Timeout: 100 * time.Millisecond}
	resp, err := client.Get("http://" + <-address + "/readyz")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("ready status=%d", resp.StatusCode)
	}
	cancel()
	if code := <-done; code != 0 {
		t.Fatalf("code=%d", code)
	}
	calls = append(calls, "ready")
	want := []string{"verified and tokenizer", "probe", "model", "warmup tokens", "warmup vector", "versions", "listen", "ready"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls %v want %v", calls, want)
	}
	// A wrong width must stop before either warmup operation.
	calls = nil
	dir = fixture(t)
	original := n.OpenModel
	n.OpenModel = func(data []byte, o onnx.Options) (httpapi.Runtime, int, io.Closer, error) {
		m, _, c, e := original(data, o)
		return m, 8, c, e
	}
	if code := Run(context.Background(), []string{"--model-dir", dir, "--ort-lib", "lib"}, io.Discard, n); code != 1 {
		t.Fatalf("width code=%d", code)
	}
	if !reflect.DeepEqual(calls, []string{"verified and tokenizer", "probe", "model"}) {
		t.Fatalf("width calls=%v", calls)
	}
}
func TestRun_DefaultListenLoopback(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var address string
	listen := func(_, addr string) (net.Listener, error) {
		address = addr
		cancel()
		return net.Listen("tcp", "127.0.0.1:0")
	}
	if code := run(ctx, []string{"--model-dir", fixture(t), "--ort-lib", "lib"}, io.Discard, natives(false, fakeModel{}), listen, nil); code != 0 {
		t.Fatalf("code=%d", code)
	}
	if address != "127.0.0.1:8091" {
		t.Fatalf("address=%q", address)
	}
}

type blockingModel struct {
	started, release chan struct{}
	once             sync.Once
}

func (m *blockingModel) Embed(_ context.Context, _ []int64) ([]float32, error) {
	blocked := false
	m.once.Do(func() { blocked = true })
	if !blocked {
		close(m.started)
		<-m.release
	}
	v := make([]float32, 1024)
	v[0] = 1
	return v, nil
}
func TestRun_DrainOnSignal(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	unready, releaseShutdown := make(chan struct{}), make(chan struct{})
	model := &blockingModel{started: make(chan struct{}), release: make(chan struct{})}
	n := natives(false, fakeModel{})
	n.OpenModel = func([]byte, onnx.Options) (httpapi.Runtime, int, io.Closer, error) { return model, 1024, closer{}, nil }
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan int, 1)
	dir := fixture(t)
	go func() {
		done <- run(ctx, []string{"--model-dir", dir, "--ort-lib", "lib"}, io.Discard, n, func(string, string) (net.Listener, error) { return l, nil }, func() { close(unready); <-releaseShutdown })
	}()
	client := &http.Client{Timeout: 100 * time.Millisecond}
	base := "http://" + l.Addr().String()
	var recipe struct {
		Hash string `json:"recipe_hash"`
	}
	for i := 0; i < 50; i++ {
		resp, e := client.Get(base + "/v1/recipe")
		if e == nil {
			_ = json.NewDecoder(resp.Body).Decode(&recipe)
			_ = resp.Body.Close()
			if resp.StatusCode == 200 {
				break
			}
		}
		time.Sleep(time.Millisecond)
	}
	if recipe.Hash == "" {
		t.Fatal("service did not become ready")
	}
	readyClient := &http.Client{Timeout: 100 * time.Millisecond, Transport: &http.Transport{}}
	resp, err := readyClient.Get(base + "/readyz")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("initial ready status=%d", resp.StatusCode)
	}
	result := make(chan int, 1)
	go func() {
		body := strings.NewReader(fmt.Sprintf(`{"recipe_hash":%q,"texts":["test"]}`, recipe.Hash))
		embedResp, e := client.Post(base+"/v1/embed", "application/json", body)
		if e != nil {
			result <- 0
			return
		}
		_ = embedResp.Body.Close()
		result <- embedResp.StatusCode
	}()
	<-model.started
	cancel()
	<-unready
	resp, err = readyClient.Get(base + "/readyz")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != 503 {
		t.Fatalf("ready status=%d", resp.StatusCode)
	}
	close(releaseShutdown)
	close(model.release)
	if status := <-result; status != 200 {
		t.Fatalf("embed status=%d", status)
	}
	if code := <-done; code != 0 {
		t.Fatalf("code=%d", code)
	}
}
