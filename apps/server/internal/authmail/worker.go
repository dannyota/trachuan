package authmail

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"danny.vn/trachuan/apps/server/internal/seal"
	"danny.vn/trachuan/apps/server/internal/store"
	"github.com/jackc/pgx/v5"
)

type Sender interface {
	Send(context.Context, Message) error
}
type Outbox interface {
	ClaimDueMail(context.Context, time.Time) (store.MailOutbox, error)
	OpenMail(context.Context, store.MailOutbox) ([]byte, error)
	FinishMail(context.Context, store.FinishMailParams) error
}
type Worker struct {
	sendTimeout time.Duration
	Store       Outbox
	Sender      Sender
	Clock       func() time.Time
	PublicURL   string
	Logger      *slog.Logger
}

func (w *Worker) Run(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		if ctx.Err() != nil {
			return
		}
		worked, err := w.Step(ctx)
		if worked && err == nil {
			continue
		}
		if err != nil && ctx.Err() == nil {
			w.Logger.Error("mail_queue_error")
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
func (w *Worker) Step(ctx context.Context) (bool, error) {
	query, cancel := context.WithTimeout(ctx, 2*time.Second)
	row, err := w.Store.ClaimDueMail(query, w.Clock())
	cancel()
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	p := store.FinishMailParams{ID: row.ID, Attempts: row.Attempts}
	result := "retry"
	switch {
	case !row.ExpiresAt.After(w.Clock()) || row.Attempts > 6:
		p.Drop = true
		result = "expired"
	default:
		query, cancel = context.WithTimeout(ctx, 2*time.Second)
		plain, openErr := w.Store.OpenMail(query, row)
		cancel()
		if errors.Is(openErr, seal.ErrUnknownKey) || errors.Is(openErr, seal.ErrAuthentication) || errors.Is(openErr, seal.ErrInvalidBinding) {
			p.Drop = true
			result = "seal_failed"
			w.Logger.Error("mail_seal_alarm", "kind", row.Kind, "row_id", row.ID, "result", result)
		} else if openErr == nil && !row.ExpiresAt.After(w.Clock()) {
			p.Drop = true
			result = "expired"
		} else if openErr == nil {
			var payload Payload
			decodeErr := json.Unmarshal(plain, &payload)
			message, renderErr := Render(w.PublicURL, row.Kind, payload)
			if decodeErr != nil || renderErr != nil {
				p.Drop = true
				result = "invalid_payload"
			} else {
				timeout := 30 * time.Second
				if w.sendTimeout > 0 {
					timeout = min(timeout, w.sendTimeout)
				}
				timeout = min(timeout, row.ExpiresAt.Sub(w.Clock()))
				sendCtx, sendCancel := context.WithTimeout(ctx, timeout)
				sendErr := w.Sender.Send(sendCtx, message)
				if sendCtx.Err() != nil {
					sendErr = sendCtx.Err()
				}
				sendCancel()
				if sendErr == nil {
					p.Sent = true
					result = "sent"
				} else if row.Attempts >= 6 {
					result = "exhausted"
				}
			}
		}
	}
	p.Now = w.Clock()
	query, cancel = context.WithTimeout(ctx, 2*time.Second)
	err = w.Store.FinishMail(query, p)
	cancel()
	if err != nil {
		return true, err
	}
	w.Logger.Info("mail_result", "kind", row.Kind, "row_id", row.ID, "result", result)
	return true, nil
}
