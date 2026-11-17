package loadrun

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type responseTransport struct{ body *countedBody }

func (t responseTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: 200, Header: make(http.Header), Body: t.body}, nil
}

type countedBody struct {
	io.Reader
	read int
}

func (b *countedBody) Read(p []byte) (int, error) {
	n, err := b.Reader.Read(p)
	b.read += n
	return n, err
}
func (b *countedBody) Close() error { return nil }
func TestSend_ReadsCompleteResponse(t *testing.T) {
	body := &countedBody{Reader: strings.NewReader(strings.Repeat("x", 2<<20))}
	client := &http.Client{Transport: responseTransport{body: body}}
	result := send(context.Background(), client, "GET", "http://127.0.0.1/document", "", "2001:db8::1", "", "", false, nil)
	if result.status != 200 || body.read != 2<<20 {
		t.Fatalf("status %d bytes %d", result.status, body.read)
	}
}

func TestSend_SelectedRequestCancelsFastResponse(t *testing.T) {
	body := &countedBody{Reader: strings.NewReader(strings.Repeat("x", 1000))}
	client := &http.Client{Transport: responseTransport{body: body}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_ = send(ctx, client, "GET", "http://127.0.0.1/document", "", "2001:db8::1", "", "", false, cancel)
	if ctx.Err() == nil {
		t.Fatal("selected request was not cancelled")
	}
}
