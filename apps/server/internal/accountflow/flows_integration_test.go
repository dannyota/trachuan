//go:build integration

package accountflow

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/migrate"
	"danny.vn/trachuan/apps/server/internal/seal"
	"danny.vn/trachuan/apps/server/internal/store"
	"danny.vn/trachuan/internal/testpg"
	"github.com/jackc/pgx/v5/pgxpool"
)

func fixture(t *testing.T) (*Service, *pgxpool.Pool) {
	t.Helper()
	dsn := testpg.NewDatabase(t)
	if err := migrate.Up(t.Context(), dsn); err != nil {
		t.Fatal(err)
	}
	db, err := pgxpool.New(t.Context(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	ring, err := seal.Parse([]byte(base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{1}, 32))))
	if err != nil {
		t.Fatal(err)
	}
	return &Service{DB: db, Ring: ring}, db
}
func requestLink(t *testing.T, s *Service, db *pgxpool.Pool, purpose, email string) string {
	t.Helper()
	if err := s.RequestMail(t.Context(), purpose, email); err != nil {
		t.Fatal(err)
	}
	var digest string
	if err := db.QueryRow(t.Context(), `SELECT token_hash FROM email_link WHERE purpose=$1 AND email=$2 AND consumed_at IS NULL ORDER BY id DESC LIMIT 1`, purpose, email).Scan(&digest); err != nil {
		t.Fatal(err)
	}
	return digest
}
func sessionParams() store.CreateSessionParams {
	return store.CreateSessionParams{TokenHash: strings.Repeat("c", 64), CsrfSecret: "synthetic", ExpiresAt: time.Now().Add(time.Hour)}
}
func TestRegistration_PurposeSingleUseAndAdmission(t *testing.T) {
	for _, refusal := range []string{"wrong purpose", "taken", "closed", "once"} {
		t.Run(refusal, func(t *testing.T) {
			s, db := fixture(t)
			digest := requestLink(t, s, db, "registration", "person@example.test")
			p := Completion{TokenHash: digest, Hash: "synthetic hash", Session: sessionParams()}
			switch refusal {
			case "wrong purpose":
				if _, err := s.CompleteReset(t.Context(), p); !errors.Is(err, ErrLinkInvalid) {
					t.Fatal("wrong purpose accepted", err)
				}
			case "taken":
				if _, err := db.Exec(t.Context(), `INSERT INTO account(google_sub,email) VALUES ('synthetic','person@example.test')`); err != nil {
					t.Fatal(err)
				}
			case "closed":
				s.SignupEmails = []string{}
			case "once":
				if _, err := s.CompleteRegistration(t.Context(), p); err != nil {
					t.Fatal(err)
				}
			}
			_, err := s.CompleteRegistration(t.Context(), p)
			if refusal == "wrong purpose" {
				if err != nil {
					t.Fatal("wrong route consumed link", err)
				}
			} else if !errors.Is(err, ErrLinkInvalid) {
				t.Fatal("refused registration created an account", err)
			}
		})
	}
}
func TestMailAdmission_RefusalDoesNotReplaceLink(t *testing.T) {
	s, db := fixture(t)
	first := requestLink(t, s, db, "registration", "person@example.test")
	second := requestLink(t, s, db, "registration", "person@example.test")
	third := requestLink(t, s, db, "registration", "person@example.test")
	if first == second || second == third {
		t.Fatal("token reused")
	}
	if err := s.RequestMail(t.Context(), "registration", "person@example.test"); err != nil {
		t.Fatal(err)
	}
	var live int
	if err := db.QueryRow(t.Context(), `SELECT count(*) FROM email_link WHERE email='person@example.test' AND consumed_at IS NULL AND token_hash=$1`, third).Scan(&live); err != nil || live != 1 {
		t.Fatal("cap changed live link", err)
	}
}
func TestReset_EpochNoticeAndSeparateAllowance(t *testing.T) {
	s, db := fixture(t)
	var id int64
	if err := db.QueryRow(t.Context(), `INSERT INTO account(google_sub,email) VALUES ('synthetic','person@example.test') RETURNING id`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(t.Context(), `INSERT INTO password_credential(account_id,hash) VALUES ($1,'old')`, id); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(t.Context(), `INSERT INTO account_security_change(account_id,kind) SELECT $1,'signed_in' FROM generate_series(1,10)`, id); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 6; i++ {
		if _, err := db.Exec(t.Context(), `UPDATE mail_outbox SET created_at=now()-interval '2 days'`); err != nil {
			t.Fatal(err)
		}
		digest := requestLink(t, s, db, "reset", "person@example.test")
		_, err := s.CompleteReset(t.Context(), Completion{TokenHash: digest, Hash: "new"})
		if i == 5 {
			if !errors.Is(err, ErrRateLimited) {
				t.Fatal("reset allowance not enforced", err)
			}
		} else if err != nil {
			t.Fatal(err)
		}
	}
	var epoch, version, notices int
	if err := db.QueryRow(context.Background(), `SELECT a.auth_epoch,p.version,(SELECT count(*) FROM mail_outbox WHERE account_id=a.id AND class='notice') FROM account a JOIN password_credential p ON p.account_id=a.id WHERE a.id=$1`, id).Scan(&epoch, &version, &notices); err != nil || epoch != 5 || version != 6 || notices != 5 {
		t.Fatalf("epoch=%d version=%d notices=%d err=%v", epoch, version, notices, err)
	}
}
