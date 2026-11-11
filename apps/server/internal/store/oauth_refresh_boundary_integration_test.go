//go:build integration

package store_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func TestStore_RefreshReuseAfterExpiryOrRevocation(t *testing.T) {
	for _, tc := range []struct {
		name, update      string
		superseded, reuse bool
	}{
		{"expired reuse", `family_expires_at = now()`, true, true},
		{"revoked reuse", `revoked_at = now()`, true, true},
		{"token expired reuse", `expires_at = now()`, true, true},
		{"expired fresh", `family_expires_at = now()`, false, false},
		{"revoked fresh", `revoked_at = now()`, false, false},
		{"token expired fresh", `expires_at = now()`, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, conn, q, _, grant := oauthFixture(t)
			hash := strings.Repeat("a", 64)
			oauthToken(t, ctx, q, grant, hash, time.Now().Add(time.Hour))
			if tc.superseded {
				if _, err := q.RotateRefresh(ctx, hash); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := conn.Exec(ctx, `UPDATE oauth_token SET `+tc.update+` WHERE token_hash = $1`, hash); err != nil {
				t.Fatal(err)
			}
			row, err := q.RotateRefresh(ctx, hash)
			if tc.reuse {
				if err != nil || row.Reuse != true {
					t.Fatalf("reuse=%v err=%v", row.Reuse, err)
				}
			} else if !errors.Is(err, pgx.ErrNoRows) {
				t.Fatalf("expected no row, got %v", err)
			}
		})
	}
}
