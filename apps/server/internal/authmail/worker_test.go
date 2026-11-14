package authmail

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/seal"
	"danny.vn/trachuan/apps/server/internal/store"
	"github.com/jackc/pgx/v5"
)

type workerStore struct {
	rows    []store.MailOutbox
	finish  []store.FinishMailParams
	openErr error
}

func (s *workerStore) ClaimDueMail(context.Context, time.Time) (store.MailOutbox, error) {
	if len(s.rows) == 0 {
		return store.MailOutbox{}, pgx.ErrNoRows
	}
	r := s.rows[0]
	s.rows = s.rows[1:]
	return r, nil
}
func (s *workerStore) OpenMail(context.Context, store.MailOutbox) ([]byte, error) {
	return []byte(`{"to":"person@example.test","token":"synthetic"}`), s.openErr
}
func (s *workerStore) FinishMail(_ context.Context, p store.FinishMailParams) error {
	s.finish = append(s.finish, p)
	return nil
}

type workerSender struct {
	calls int
	err   error
}

func (s *workerSender) Send(ctx context.Context, _ Message) error {
	s.calls++
	if _, ok := ctx.Deadline(); !ok {
		return errors.New("no deadline")
	}
	return s.err
}

func TestWorker_SuccessRetryExpiryAndAlarm(t *testing.T) {
	now := time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name             string
		attempt          int32
		expired          bool
		openErr, sendErr error
		sent, drop       bool
		uncertain        bool
		calls            int
	}{
		{name: "success", attempt: 1, sent: true, calls: 1},
		{name: "retry", attempt: 1, uncertain: true, sendErr: errors.New("private SMTP detail"), calls: 1},
		{name: "last attempt", attempt: 6, uncertain: true, sendErr: errors.New("private detail"), calls: 1},
		{name: "definite failure", attempt: 1, sendErr: ErrNotDelivered, calls: 1},
		{name: "expired", attempt: 2, expired: true, drop: true},
		{name: "unknown key", attempt: 1, openErr: seal.ErrUnknownKey, drop: true},
		{name: "failed open", attempt: 1, openErr: seal.ErrAuthentication, drop: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			expiry := now.Add(time.Hour)
			if tc.expired {
				expiry = now
			}
			s := &workerStore{rows: []store.MailOutbox{{ID: 123, Kind: "reset", Attempts: tc.attempt, ExpiresAt: expiry}}, openErr: tc.openErr}
			sender := &workerSender{err: tc.sendErr}
			var logs bytes.Buffer
			w := Worker{Store: s, Sender: sender, Clock: func() time.Time { return now }, PublicURL: "https://example.test", Logger: slog.New(slog.NewJSONHandler(&logs, nil))}
			worked, err := w.Step(context.Background())
			if !worked || err != nil || len(s.finish) != 1 || sender.calls != tc.calls {
				t.Fatal("worker failed", err)
			}
			p := s.finish[0]
			if p.Sent != tc.sent || p.Drop != tc.drop || p.Attempts != tc.attempt || p.Uncertain != tc.uncertain {
				t.Fatal("wrong finish state")
			}
			if strings.Contains(logs.String(), "person@example.test") || strings.Contains(logs.String(), "synthetic") || strings.Contains(logs.String(), "private") {
				t.Fatal("private data logged")
			}
			if tc.openErr != nil && !strings.Contains(logs.String(), "mail_seal_alarm") {
				t.Fatal("missing alarm event")
			}
		})
	}
}

type stallingSender struct{ calls int }

func (s *stallingSender) Send(ctx context.Context, _ Message) error {
	s.calls++
	if s.calls == 1 {
		<-ctx.Done()
		return ctx.Err()
	}
	return nil
}
func TestWorker_StalledMessageAllowsNextSend(t *testing.T) {
	now := time.Now()
	s := &workerStore{rows: []store.MailOutbox{{ID: 1, Kind: "reset", Attempts: 1, ExpiresAt: now.Add(time.Hour)}, {ID: 2, Kind: "reset", Attempts: 1, ExpiresAt: now.Add(time.Hour)}}}
	sender := &stallingSender{}
	var logs bytes.Buffer
	w := Worker{Store: s, Sender: sender, Clock: func() time.Time { return now }, PublicURL: "https://example.test", Logger: slog.New(slog.NewJSONHandler(&logs, nil)), sendTimeout: time.Millisecond}
	for range 2 {
		if worked, err := w.Step(context.Background()); !worked || err != nil {
			t.Fatal("worker stopped", err)
		}
	}
	if sender.calls != 2 || s.finish[0].Sent || !s.finish[1].Sent {
		t.Fatal("stalled message blocked later mail")
	}
}
