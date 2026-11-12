package ocrvision

import (
	"bytes"
	"context"
	"crypto/tls"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/hpack"
	"golang.org/x/oauth2"
)

func TestClient_HTTP2RefusalCannotReplayOutsideBudget(t *testing.T) {
	var streams, waits atomic.Int64
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Error("HTTP/1 fallback") }))
	server.EnableHTTP2 = true
	server.Config.TLSNextProto = map[string]func(*http.Server, *tls.Conn, http.Handler){"h2": func(_ *http.Server, conn *tls.Conn, _ http.Handler) {
		defer func() { _ = conn.Close() }()
		preface := make([]byte, len(http2.ClientPreface))
		if _, err := io.ReadFull(conn, preface); err != nil {
			return
		}
		if string(preface) != http2.ClientPreface {
			t.Error("missing HTTP/2 preface")
			return
		}
		frames := http2.NewFramer(conn, conn)
		if err := frames.WriteSettings(); err != nil {
			return
		}
		for {
			frame, err := frames.ReadFrame()
			if err != nil {
				return
			}
			switch f := frame.(type) {
			case *http2.SettingsFrame:
				if !f.IsAck() {
					if err = frames.WriteSettingsAck(); err != nil {
						return
					}
				}
			case *http2.HeadersFrame:
				if streams.Add(1) == 1 {
					if err = frames.WriteRSTStream(f.StreamID, http2.ErrCodeRefusedStream); err != nil {
						return
					}
				} else {
					var headers bytes.Buffer
					encoder := hpack.NewEncoder(&headers)
					if err = encoder.WriteField(hpack.HeaderField{Name: ":status", Value: "200"}); err != nil {
						return
					}
					if err = frames.WriteHeaders(http2.HeadersFrameParam{StreamID: f.StreamID, BlockFragment: headers.Bytes(), EndHeaders: true}); err != nil {
						return
					}
					if err = frames.WriteData(f.StreamID, true, []byte(`{"responses":[{}]}`)); err != nil {
						return
					}
				}
			}
		}
	}}
	server.StartTLS()
	defer server.Close()
	client := NewClient(Config{BaseURL: server.URL, HTTP: server.Client(), Project: "test-project", TokenSource: oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "test-token"}), Wait: func(context.Context) error { waits.Add(1); return nil }, Sleep: func(context.Context, time.Duration) error { return nil }})
	budget := newBudget(1)
	_, err := client.recognize(context.Background(), []byte("image"), budget)
	if err == nil || streams.Load() != 1 || budget.used() != 1 || waits.Load() != 1 {
		t.Fatalf("err=%v streams=%d debits=%d waits=%d", err, streams.Load(), budget.used(), waits.Load())
	}
}
