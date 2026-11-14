package authmail

import (
	"bytes"
	"context"
	"crypto/x509"
	"errors"
	"io"
	"log/slog"
	"net/textproto"
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/store"
)

func TestSMTP_WorkerPreservesOnlyUncertainDeliveryReservations(t *testing.T) {
	for _, tc := range []struct {
		mode      string
		uncertain bool
	}{
		{"dial", false}, {"no TLS", false}, {"TLS rejected", false}, {"untrusted", false}, {"wrong host", false},
		{"AUTH rejected", false}, {"MAIL rejected", false}, {"RCPT rejected", false}, {"stall", false},
		{"DATA rejected", false}, {"DATA body rejected 554", false}, {"DATA body rejected 451", false},
		{"DATA malformed", true}, {"DATA body malformed", true}, {"DATA command disconnect", true}, {"DATA disconnect", true}, {"DATA acknowledgement lost", true}, {"DATA deadline", true},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			opts, commands, dataStarted := smtpTestServer(t, tc.mode)
			if tc.mode == "untrusted" {
				opts.RootCAs = x509.NewCertPool()
			}
			if tc.mode == "wrong host" {
				opts.serverName = "other.example.test"
			}
			sender, err := NewSMTP(opts)
			if err != nil {
				t.Fatal(err)
			}
			now := time.Now()
			outbox := &workerStore{rows: []store.MailOutbox{{ID: 1, Kind: "reset", Attempts: 1, ExpiresAt: now.Add(time.Hour)}}}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			var logs bytes.Buffer
			worker := Worker{Store: outbox, Sender: sender, Clock: func() time.Time { return now }, PublicURL: "https://example.test", Logger: slog.New(slog.NewTextHandler(&logs, nil)), sendTimeout: 80 * time.Millisecond}
			if worked, stepErr := worker.Step(ctx); !worked || stepErr != nil {
				t.Fatal("worker failed", stepErr)
			}
			<-commands
			if tc.mode == "DATA deadline" {
				select {
				case <-dataStarted:
				default:
					t.Fatal("deadline occurred before DATA")
				}
			}
			if len(outbox.finish) != 1 || outbox.finish[0].Sent || outbox.finish[0].Uncertain != tc.uncertain {
				t.Fatalf("reservation uncertain=%v, want=%v", outbox.finish, tc.uncertain)
			}
		})
	}
}

type submissionTransport struct {
	input    io.Reader
	writeErr error
	written  bytes.Buffer
}

func (c *submissionTransport) Read(p []byte) (int, error) { return c.input.Read(p) }
func (c *submissionTransport) Write(p []byte) (int, error) {
	if c.writeErr != nil {
		return 0, c.writeErr
	}
	return c.written.Write(p)
}
func (*submissionTransport) Close() error { return nil }
func TestDelivery_ClassifiesCommandWriteAndReplies(t *testing.T) {
	for _, tc := range []struct {
		name, replies     string
		writeErr          error
		definite, success bool
	}{
		{name: "command write failed", writeErr: io.ErrClosedPipe, definite: true},
		{name: "command reply missing"},
		{name: "command reply malformed", replies: "invalid reply\r\n"},
		{name: "command rejected", replies: "554 Rejected\r\n", definite: true},
		{name: "message reply missing", replies: "354 Continue\r\n"},
		{name: "message rejected 554", replies: "354 Continue\r\n554 Rejected\r\n", definite: true},
		{name: "message rejected 451", replies: "354 Continue\r\n451 Retry\r\n", definite: true},
		{name: "message reply malformed", replies: "354 Continue\r\ninvalid reply\r\n"},
		{name: "unexpected positive reply", replies: "354 Continue\r\n251 Unexpected\r\n"},
		{name: "success", replies: "354 Continue\r\n250 Accepted\r\n", success: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			transport := &submissionTransport{input: strings.NewReader(tc.replies), writeErr: tc.writeErr}
			err := submitMessage(textproto.NewConn(transport), []byte("Synthetic\r\n"))
			if (err == nil) != tc.success || errors.Is(err, ErrNotDelivered) != tc.definite {
				t.Fatalf("err=%v definite=%v want=%v", err, errors.Is(err, ErrNotDelivered), tc.definite)
			}
			if tc.writeErr != nil && !errors.Is(err, tc.writeErr) {
				t.Fatal("lost command write error", err)
			}
			if tc.writeErr == nil && !strings.HasPrefix(transport.written.String(), "DATA\r\n") {
				t.Fatal("DATA command was not written")
			}
		})
	}
}
