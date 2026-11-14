//go:build integration

package store_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/textproto"
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/authmail"
	"danny.vn/trachuan/apps/server/internal/seal"
	"danny.vn/trachuan/apps/server/internal/store"
	"github.com/jackc/pgx/v5"
)

type deliveryBudgetOutbox struct {
	*store.Queries
	ring *seal.Ring
}

func (s deliveryBudgetOutbox) OpenMail(ctx context.Context, row store.MailOutbox) ([]byte, error) {
	return s.OpenClaimedMail(ctx, s.ring, row)
}

type deliveryBudgetSender struct{ err error }

func (s deliveryBudgetSender) Send(context.Context, authmail.Message) error { return s.err }
func TestMail_WorkerChargesOnlyUncertainFailures(t *testing.T) {
	for _, tc := range []struct {
		name     string
		sendErr  error
		reserved int
	}{
		{"definite non-delivery", authmail.ErrNotDelivered, 99},
		{"DATA command rejected", fmt.Errorf("%w: %w", authmail.ErrNotDelivered, &textproto.Error{Code: 554, Msg: "Rejected"}), 99},
		{"message rejected 554", fmt.Errorf("%w: %w", authmail.ErrNotDelivered, &textproto.Error{Code: 554, Msg: "Rejected"}), 99},
		{"message rejected 451", fmt.Errorf("%w: %w", authmail.ErrNotDelivered, &textproto.Error{Code: 451, Msg: "Retry"}), 99},
		{"acknowledgement missing", io.EOF, 100},
		{"unparseable reply", textproto.ProtocolError("invalid reply"), 100},
		{"deadline after DATA", context.DeadlineExceeded, 100},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, conn, q := setup(t)
			id := webAccount(t, ctx, conn, "mail-delivery")
			ring, err := seal.Parse([]byte(strings.Repeat("A", 43)))
			if err != nil {
				t.Fatal(err)
			}
			tx, err := conn.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = tx.Rollback(ctx) }()
			for range 2 {
				_, err = store.EnqueueMail(ctx, tx, ring, store.EnqueueMailParams{Kind: "reset", Class: "recovery", AccountID: &id, Payload: []byte(`{"to":"mail-delivery@example.test","token":"synthetic"}`)})
				if err != nil {
					t.Fatal(err)
				}
			}
			if err = tx.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			now := time.Date(2099, 6, 20, 12, 0, 0, 0, time.UTC)
			if _, err = conn.Exec(ctx, `UPDATE mail_outbox SET next_attempt_at=$1,expires_at=$1::timestamptz+interval '1 day'`, now); err != nil {
				t.Fatal(err)
			}
			if _, err = conn.Exec(ctx, `INSERT INTO mail_delivery(outbox_id,attempt,class,reserved_at,sent_at) SELECT -g,1,'recovery',$1,$1 FROM generate_series(1,99) g`, now); err != nil {
				t.Fatal(err)
			}
			worker := authmail.Worker{Store: deliveryBudgetOutbox{q, ring}, Sender: deliveryBudgetSender{tc.sendErr}, Clock: func() time.Time { return now }, PublicURL: "https://example.test", Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
			if worked, stepErr := worker.Step(ctx); !worked || stepErr != nil {
				t.Fatal("worker failed", stepErr)
			}
			var reserved int
			if err = conn.QueryRow(ctx, `SELECT count(*) FROM mail_delivery`).Scan(&reserved); err != nil || reserved != tc.reserved {
				t.Fatalf("reservations=%d want=%d err=%v", reserved, tc.reserved, err)
			}
			_, err = q.ClaimDueMail(ctx, now)
			if tc.reserved == 100 {
				if !errors.Is(err, pgx.ErrNoRows) {
					t.Fatal("uncertain delivery exceeded budget", err)
				}
			} else if err != nil {
				t.Fatal("definite failure blocked next message", err)
			}
		})
	}
}
