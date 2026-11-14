package authmail

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"mime"
	"mime/quotedprintable"
	"net"
	"net/smtp"
	"net/textproto"
	"strconv"
	"strings"
	"time"

	"danny.vn/trachuan/apps/server/internal/accountemail"
)

type SMTPOptions struct {
	Host, Port, Username, Password, From string
	RootCAs                              *x509.CertPool
	serverName                           string
}
type SMTP struct{ options SMTPOptions }

func NewSMTP(opts SMTPOptions) (*SMTP, error) {
	port, err := strconv.Atoi(opts.Port)
	_, fromErr := accountemail.Parse(opts.From)
	if opts.Host == "" || strings.ContainsAny(opts.Host, "\r\n /\\") || err != nil || port < 1 || port > 65535 || opts.Username == "" || opts.Password == "" || strings.ContainsAny(opts.Username+opts.Password, "\r\n\x00") || fromErr != nil {
		return nil, errors.New("invalid SMTP configuration")
	}
	return &SMTP{options: opts}, nil
}
func (s *SMTP) Send(ctx context.Context, msg Message) error {
	data, err := renderMessage(s.options.From, msg)
	if err != nil {
		return err
	}
	if err = s.deliver(ctx, msg.To, data); err != nil {
		return fmt.Errorf("SMTP delivery: %w", err)
	}
	return nil
}
func (s *SMTP) deliver(ctx context.Context, to string, data []byte) error {
	deadline := time.Now().Add(30 * time.Second)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	dialer := net.Dialer{Deadline: deadline}
	conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(s.options.Host, s.options.Port))
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()
	if err = conn.SetDeadline(deadline); err != nil {
		return err
	}
	stop := context.AfterFunc(ctx, func() { _ = conn.SetDeadline(time.Unix(1, 0)) })
	defer stop()
	client, err := smtp.NewClient(conn, s.options.Host)
	if err != nil {
		return err
	}
	if ok, _ := client.Extension("STARTTLS"); !ok {
		return errors.New("SMTP STARTTLS required")
	}
	serverName := s.options.Host
	if s.options.serverName != "" {
		serverName = s.options.serverName
	}
	if err = client.StartTLS(&tls.Config{ServerName: serverName, RootCAs: s.options.RootCAs, MinVersion: tls.VersionTLS12}); err != nil {
		return err
	}
	if _, ok := client.TLSConnectionState(); !ok {
		return errors.New("SMTP TLS required")
	}
	if err = client.Auth(smtp.PlainAuth("", s.options.Username, s.options.Password, s.options.Host)); err != nil {
		return err
	}
	if err = client.Mail(s.options.From); err != nil {
		return err
	}
	if err = client.Rcpt(to); err != nil {
		return err
	}
	writer, err := client.Data()
	if err != nil {
		return err
	}
	if _, err = writer.Write(data); err != nil {
		return err
	}
	if err = writer.Close(); err != nil {
		var reply *textproto.Error
		if !errors.As(err, &reply) || reply.Code < 200 || reply.Code >= 300 {
			return err
		}
	}
	_ = client.Quit()
	return nil
}
func renderMessage(from string, msg Message) ([]byte, error) {
	if _, err := accountemail.Parse(msg.To); err != nil || strings.ContainsAny(msg.To+msg.Subject+from, "\r\n") {
		return nil, errors.New("invalid mail headers")
	}
	var body bytes.Buffer
	writer := quotedprintable.NewWriter(&body)
	if _, err := writer.Write([]byte(msg.Text)); err != nil {
		return nil, fmt.Errorf("encode mail body: %w", err)
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("finish mail body: %w", err)
	}
	headers := []string{"From: " + from, "To: " + msg.To, "Subject: " + mime.QEncoding.Encode("utf-8", msg.Subject), "Date: " + time.Now().UTC().Format(time.RFC1123Z), "MIME-Version: 1.0", "Content-Type: text/plain; charset=UTF-8", "Content-Transfer-Encoding: quoted-printable"}
	return append([]byte(strings.Join(headers, "\r\n")+"\r\n\r\n"), body.Bytes()...), nil
}
