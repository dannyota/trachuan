package httpapi

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/apps/queryembed/internal/admission"
)

func TestPost_NULText400(t *testing.T) {
	s, rt, r, _ := setup(t)
	s.SetReady(true)
	for _, path := range []string{"/v1/tokenize", "/v1/embed"} {
		w := post(s, path, body(r, "a\x00b"))
		if w.Code != 400 || !strings.Contains(w.Body.String(), `"code":"invalid_request"`) {
			t.Errorf("%s: %d %s", path, w.Code, w.Body)
		}
	}
	if rt.calls.Load() != 0 {
		t.Fatal("runtime called for invalid text")
	}
}

type blockingTokenizer struct{ started, release chan struct{} }

func (b blockingTokenizer) Encode(string) ([]int64, error) {
	close(b.started)
	<-b.release
	return []int64{1}, nil
}

func TestPost_CancelledDuringTokenization(t *testing.T) {
	s, rt, r, _ := setup(t)
	b := blockingTokenizer{make(chan struct{}), make(chan struct{})}
	s.cfg.Tokenizer = b
	s.cfg.Admission = admission.New(admission.Limits{Active: 1, Wait: time.Millisecond * 10, Deadline: time.Millisecond * 10})
	s.SetReady(true)
	done := make(chan int, 1)
	go func() { done <- post(s, "/v1/embed", body(r, "x")).Code }()
	deadline := time.After(80 * time.Millisecond)
	select {
	case <-b.started:
	case <-deadline:
		t.Fatal("tokenizer did not start")
	}
	select {
	case status := <-done:
		if status != 504 {
			t.Errorf("status = %d", status)
		}
	case <-deadline:
		close(b.release)
		t.Fatal("request did not time out")
	}
	close(b.release)
	// The slot is released only after the tokenizer returns and its work ends.
	for {
		err := s.cfg.Admission.Do(context.Background(), func(context.Context) error { return nil })
		if err == nil {
			break
		}
		select {
		case <-deadline:
			t.Fatal("work did not finish")
		default:
		}
	}
	if rt.calls.Load() != 0 {
		t.Fatal("runtime called after cancellation")
	}
}

func realPost(t *testing.T, bodyPrefix string, contentLength int, ready bool) (*http.Response, *bufio.Reader) {
	t.Helper()
	s, _, _, _ := setup(t)
	s.SetReady(ready)
	returned := make(chan struct{})
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 20*time.Millisecond)
		defer cancel()
		s.Handler().ServeHTTP(w, r.WithContext(ctx))
		close(returned)
	}))
	server.Start()
	t.Cleanup(func() { go server.Close() })
	conn, err := net.Dial("tcp", server.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if err = conn.SetDeadline(time.Now().Add(90 * time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	if _, err = fmt.Fprintf(conn, "POST /v1/embed HTTP/1.1\r\nHost: localhost\r\nContent-Type: application/json\r\nContent-Length: %d\r\n\r\n%s", contentLength, bodyPrefix); err != nil {
		t.Fatal(err)
	}
	reader := bufio.NewReader(conn)
	resp, err := http.ReadResponse(reader, nil)
	if err != nil {
		t.Fatalf("reading response: %v", err)
	}
	select {
	case <-returned:
	case <-time.After(50 * time.Millisecond):
		t.Fatal("handler did not return")
	}
	return resp, reader
}

func TestPost_RealBodyDeadline504(t *testing.T) {
	resp, _ := realPost(t, "", 10, true)
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(resp.Body)
	if err != nil || resp.StatusCode != 504 || !strings.Contains(string(b), `"code":"deadline_exceeded"`) {
		t.Fatalf("status %d, body %s, error %v", resp.StatusCode, b, err)
	}
}

func TestPost_OversizeStalledBodyCloses(t *testing.T) {
	resp, reader := realPost(t, strings.Repeat("x", 65537), 131072, true)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 413 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if _, err := io.ReadAll(resp.Body); err != nil {
		t.Fatal(err)
	}
	if _, err := reader.ReadByte(); err != io.EOF {
		t.Fatalf("connection remained open: %v", err)
	}
}

func TestPost_NotReadyStalledBodyCloses(t *testing.T) {
	resp, reader := realPost(t, "", 131072, false)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 503 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if _, err := io.ReadAll(resp.Body); err != nil {
		t.Fatal(err)
	}
	if _, err := reader.ReadByte(); err != io.EOF {
		t.Fatalf("connection remained open: %v", err)
	}
}
