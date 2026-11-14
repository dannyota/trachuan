package authmail

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"io"
	"net"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func smtpTestServer(t *testing.T, mode string) (SMTPOptions, <-chan []string) {
	t.Helper()
	certServer := httptest.NewTLSServer(nil)
	certificate := certServer.TLS.Certificates[0]
	roots := x509.NewCertPool()
	roots.AddCert(certServer.Certificate())
	certServer.Close()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	result := make(chan []string, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		var commands []string
		defer func() { result <- commands }()
		reader := bufio.NewReader(conn)
		if mode == "stall" {
			_, _ = reader.ReadString('\n')
			return
		}
		_, _ = io.WriteString(conn, "220 localhost\r\n")
		secure := false
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				return
			}
			command := strings.TrimSpace(line)
			commands = append(commands, command)
			switch {
			case strings.HasPrefix(command, "EHLO"):
				if mode == "no TLS" {
					_, _ = io.WriteString(conn, "250-localhost\r\n250 AUTH PLAIN\r\n")
				} else if secure {
					_, _ = io.WriteString(conn, "250-localhost\r\n250 AUTH PLAIN\r\n")
				} else {
					_, _ = io.WriteString(conn, "250-localhost\r\n250-STARTTLS\r\n250 AUTH PLAIN\r\n")
				}
			case command == "STARTTLS":
				_, _ = io.WriteString(conn, "220 Ready\r\n")
				transport := tls.Server(conn, &tls.Config{Certificates: []tls.Certificate{certificate}, MinVersion: tls.VersionTLS12})
				if err := transport.Handshake(); err != nil {
					return
				}
				conn = transport
				reader = bufio.NewReader(conn)
				secure = true
			case strings.HasPrefix(command, "AUTH"):
				if !secure {
					_, _ = io.WriteString(conn, "530 TLS required\r\n")
					return
				}
				_, _ = io.WriteString(conn, "235 OK\r\n")
			case command == "DATA":
				_, _ = io.WriteString(conn, "354 Go ahead\r\n")
				for {
					s, err := reader.ReadString('\n')
					if err != nil {
						return
					}
					if s == ".\r\n" {
						break
					}
				}
				_, _ = io.WriteString(conn, "250 Queued\r\n")
			case command == "QUIT":
				_, _ = io.WriteString(conn, "221 Bye\r\n")
				return
			default:
				_, _ = io.WriteString(conn, "250 OK\r\n")
			}
		}
	}()
	host, port, _ := net.SplitHostPort(listener.Addr().String())
	return SMTPOptions{Host: host, Port: port, Username: "synthetic", Password: "synthetic", From: "no-reply@example.test", RootCAs: roots}, result
}

func TestSMTP_STARTTLSAndDeadline(t *testing.T) {
	for _, mode := range []string{"TLS", "no TLS", "untrusted", "wrong host", "stall"} {
		t.Run(mode, func(t *testing.T) {
			opts, result := smtpTestServer(t, mode)
			if mode == "untrusted" {
				opts.RootCAs = x509.NewCertPool()
			}
			if mode == "wrong host" {
				opts.serverName = "other.example.test"
			}
			sender, err := NewSMTP(opts)
			if err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()
			cancel := func() {}
			if mode == "stall" {
				ctx, cancel = context.WithTimeout(ctx, 20*time.Millisecond)
			}
			defer cancel()
			err = sender.Send(ctx, Message{To: "person@example.test", Subject: "Synthetic", Text: "Synthetic"})
			if (err == nil) != (mode == "TLS") {
				t.Fatalf("unexpected result %v", err)
			}
			commands := <-result
			startTLS, auth := -1, -1
			for i, c := range commands {
				if c == "STARTTLS" {
					startTLS = i
				}
				if strings.HasPrefix(c, "AUTH") {
					auth = i
				}
			}
			if mode == "TLS" && (startTLS < 0 || auth <= startTLS) {
				t.Fatal("AUTH before TLS")
			}
			if mode != "TLS" && auth >= 0 {
				t.Fatal("credentials sent without verified TLS")
			}
		})
	}
}
