package app

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"time"

	"danny.vn/trachuan/apps/server/internal/authmail"
	"danny.vn/trachuan/apps/server/internal/seal"
	"danny.vn/trachuan/apps/server/internal/store"
)

type mailStore struct {
	*store.Queries
	ring *seal.Ring
}

func (s mailStore) OpenMail(ctx context.Context, row store.MailOutbox) ([]byte, error) {
	return s.OpenClaimedMail(ctx, s.ring, row)
}
func startMail(ctx context.Context, cfg config, q *store.Queries, stderr io.Writer) (func(), error) {
	logger := slog.New(slog.NewJSONHandler(stderr, nil))
	var sender authmail.Sender
	var err error
	switch {
	case cfg.mail.capture != "":
		sender, err = authmail.NewCapture(cfg.mail.capture)
	case cfg.mail.smtp.Host != "":
		sender, err = authmail.NewSMTP(cfg.mail.smtp)
	default:
		logger.Warn("mail_disabled")
		return func() {}, nil
	}
	if err != nil {
		return nil, errors.New("mail sender initialization failed")
	}
	workerCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	worker := authmail.Worker{Store: mailStore{Queries: q, ring: cfg.accountKeys}, Sender: sender, Clock: time.Now, PublicURL: cfg.publicURL, Logger: logger}
	go func() { defer close(done); worker.Run(workerCtx) }()
	return func() { cancel(); <-done }, nil
}
