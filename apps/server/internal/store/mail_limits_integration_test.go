//go:build integration

package store_test

import (
	"errors"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/store"
	"github.com/jackc/pgx/v5"
)

func TestMail_BudgetDefersWithoutClaimingOrDropping(t *testing.T) {
	ctx, conn, q := setup(t)
	id := webAccount(t, ctx, conn, "mail-budget")
	now := time.Now().UTC().Truncate(time.Microsecond)
	if _, err := conn.Exec(ctx, `INSERT INTO mail_outbox(kind,class,account_id,sealed_payload,key_id,next_attempt_at,expires_at) VALUES ('registration','signup',$1,'synthetic','synthetic',$2,$2::timestamptz+interval '1 day')`, id, now); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, `INSERT INTO mail_delivery(outbox_id,attempt,class,reserved_at,sent_at) SELECT -g,1,'signup',$1,$1 FROM generate_series(1,60) g`, now); err != nil {
		t.Fatal(err)
	}
	if _, err := q.ClaimDueMail(ctx, now); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("signup share exceeded", err)
	}
	var attempts int
	var payload []byte
	if err := conn.QueryRow(ctx, `SELECT attempts,sealed_payload FROM mail_outbox`).Scan(&attempts, &payload); err != nil || attempts != 0 || len(payload) == 0 {
		t.Fatal("deferred row changed", err)
	}
	if _, err := conn.Exec(ctx, `INSERT INTO mail_outbox(kind,class,account_id,sealed_payload,key_id,next_attempt_at,expires_at) VALUES ('reset','recovery',$1,'synthetic','synthetic',$2,$2::timestamptz+interval '1 day')`, id, now); err != nil {
		t.Fatal(err)
	}
	row, err := q.ClaimDueMail(ctx, now)
	if err != nil || row.Class != "recovery" {
		t.Fatal("signup blocked reserved share", err)
	}
	if err = q.FinishMail(ctx, store.FinishMailParams{ID: row.ID, Attempts: row.Attempts, Now: now, Sent: true}); err != nil {
		t.Fatal(err)
	}
	if _, err = q.ClaimDueMail(ctx, now.Add(time.Hour)); err != nil {
		t.Fatal("hour did not renew", err)
	}
}
func TestMail_MonthlyBudgetSurvivesOutboxDeletion(t *testing.T) {
	ctx, conn, q := setup(t)
	id := webAccount(t, ctx, conn, "mail-month")
	now := time.Date(2099, 6, 20, 12, 0, 0, 0, time.UTC)
	if _, err := conn.Exec(ctx, `INSERT INTO mail_delivery(outbox_id,attempt,class,reserved_at,sent_at) SELECT -g,1,'recovery',$1,$1 FROM generate_series(1,2500) g`, now.Add(-48*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, `INSERT INTO mail_outbox(kind,class,account_id,sealed_payload,key_id,next_attempt_at,expires_at) VALUES ('password_changed','notice',$1,'synthetic','synthetic',$2,$2::timestamptz+interval '40 days')`, id, now); err != nil {
		t.Fatal(err)
	}
	if _, err := q.ClaimDueMail(ctx, now); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("monthly budget lost", err)
	}
	if _, err := q.ClaimDueMail(ctx, time.Date(2099, 7, 1, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal("calendar month did not renew", err)
	}
}

func TestMail_GlobalHourlyAndSignupMonthlyBudgets(t *testing.T) {
	for _, tc := range []struct {
		name, class string
		count       int
		monthly     bool
	}{
		{"all classes hourly", "notice", 100, false},
		{"signup monthly", "signup", 1500, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, conn, q := setup(t)
			id := webAccount(t, ctx, conn, "mail-budget")
			now := time.Date(2099, 6, 20, 12, 0, 0, 0, time.UTC)
			sent := now
			if tc.monthly {
				sent = now.Add(-48 * time.Hour)
			}
			if _, err := conn.Exec(ctx, `INSERT INTO mail_delivery(outbox_id,attempt,class,reserved_at,sent_at) SELECT -g,1,$1,$2,$2 FROM generate_series(1,$3::integer) g`, tc.class, sent, tc.count); err != nil {
				t.Fatal(err)
			}
			kind := "password_changed"
			if tc.class == "signup" {
				kind = "registration"
			}
			if _, err := conn.Exec(ctx, `INSERT INTO mail_outbox(kind,class,account_id,sealed_payload,key_id,next_attempt_at,expires_at) VALUES ($1,$2,$3,'synthetic','synthetic',$4,$4::timestamptz+interval '40 days')`, kind, tc.class, id, now); err != nil {
				t.Fatal(err)
			}
			if _, err := q.ClaimDueMail(ctx, now); !errors.Is(err, pgx.ErrNoRows) {
				t.Fatal("budget exceeded", err)
			}
			next := now.Add(time.Hour)
			if tc.monthly {
				next = time.Date(2099, 7, 1, 0, 0, 0, 0, time.UTC)
			}
			if _, err := q.ClaimDueMail(ctx, next); err != nil {
				t.Fatal("budget did not renew", err)
			}
		})
	}
}

func TestMail_UncertainDeliveryKeepsBudgetReservation(t *testing.T) {
	ctx, conn, q := setup(t)
	id := webAccount(t, ctx, conn, "uncertain-mail")
	now := time.Date(2099, 6, 20, 12, 0, 0, 0, time.UTC)
	if _, err := conn.Exec(ctx, `INSERT INTO mail_delivery(outbox_id,attempt,class,reserved_at,sent_at) SELECT -g,1,'recovery',$1,$1 FROM generate_series(1,99) g`, now); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, `INSERT INTO mail_outbox(kind,class,account_id,sealed_payload,key_id,next_attempt_at,expires_at) SELECT 'reset','recovery',$1,'synthetic','synthetic',$2,$2::timestamptz+interval '1 day' FROM generate_series(1,2)`, id, now); err != nil {
		t.Fatal(err)
	}
	row, err := q.ClaimDueMail(ctx, now)
	if err != nil {
		t.Fatal(err)
	}
	if err = q.FinishMail(ctx, store.FinishMailParams{ID: row.ID, Attempts: row.Attempts, Now: now, Uncertain: true}); err != nil {
		t.Fatal(err)
	}
	if _, err = q.ClaimDueMail(ctx, now); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("uncertain delivery released budget", err)
	}
}
