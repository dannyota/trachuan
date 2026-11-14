package admin

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestAdmin_CreateAndBlockAccount(t *testing.T) {
	a, s, _ := service()
	account, err := a.CreateAccount(context.Background(), "subject", "test@example.invalid")
	if err != nil || account.ID != 7 || s.accountParams.GoogleSub != "subject" || s.accountParams.Email != "test@example.invalid" {
		t.Fatalf("create account: %v", err)
	}
	if err := a.BlockAccount(context.Background(), 7); err != nil || s.blocked != 7 {
		t.Fatalf("block account: %v", err)
	}
}

func TestAdmin_StoreErrorsAreSafe(t *testing.T) {
	a, s, _ := service()
	s.err = errors.New("sensitive query and credential")
	_, createErr := a.CreateAccount(context.Background(), "subject", "test@example.invalid")
	for _, err := range []error{createErr, a.BlockAccount(context.Background(), 7), a.RevokeToken(context.Background(), strings.Repeat("a", 64))} {
		if err == nil || strings.Contains(err.Error(), "sensitive") {
			t.Fatal("database error details escaped")
		}
	}
}

func TestAdmin_CreateAccountClassifiesTakenEmail(t *testing.T) {
	emailConflict := &pgconn.PgError{Code: "23505", ConstraintName: "account_email_key", Message: "synthetic database detail", Detail: "Key (email)=(taken@example.test) already exists."}
	for _, tc := range []struct {
		name  string
		cause error
		taken bool
	}{
		{"email", emailConflict, true},
		{"wrapped email", fmt.Errorf("insert account: %w", emailConflict), true},
		{"Google subject", &pgconn.PgError{Code: "23505", ConstraintName: "account_google_sub_key"}, false},
		{"other constraint", &pgconn.PgError{Code: "23505", ConstraintName: "other_key"}, false},
		{"other SQLSTATE", &pgconn.PgError{Code: "23514", ConstraintName: "account_email_key"}, false},
		{"unavailable", errors.New("synthetic database detail"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, s, _ := service()
			s.err = tc.cause
			account, err := a.CreateAccount(context.Background(), "subject", "taken@example.test")
			if err == nil || account.ID != 0 {
				t.Fatal("failed creation returned an account")
			}
			if tc.taken {
				if !strings.Contains(err.Error(), "email already belongs to an account") || errors.Is(err, errStore) {
					t.Fatalf("operator error=%v", err)
				}
			} else if !errors.Is(err, errStore) {
				t.Fatalf("store error misclassified: %v", err)
			}
			if strings.Contains(err.Error(), "taken@example.test") || strings.Contains(err.Error(), "synthetic database detail") {
				t.Fatal("database detail leaked")
			}
		})
	}
}
