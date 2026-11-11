package admin

import (
	"context"
	"errors"
	"strings"
	"testing"
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
