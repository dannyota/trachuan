package admin

import (
	"context"
	"errors"
	"testing"

	"danny.vn/trachuan/apps/server/internal/store"
)

func TestAdmin_EnrolledRefusalNamesOperatorLimit(t *testing.T) {
	service, s, _ := service()
	s.createErr = store.ErrFactorEnrolled
	token, err := service.CreateToken(context.Background(), 7, "synthetic", 0)
	if token != "" || !errors.Is(err, store.ErrFactorEnrolled) || err.Error() != "personal tokens cannot be issued by the operator for an account with an enrolled factor" {
		t.Fatalf("token issued=%v error=%v", token != "", err)
	}
}
