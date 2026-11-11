//go:build integration

package store_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/store"
)

func TestStore_AccountActionsRevokeOAuth(t *testing.T) {
	for _, tc := range []struct {
		name string
		act  func(context.Context, *store.Queries, int64) error
	}{
		{"block", func(ctx context.Context, q *store.Queries, id int64) error {
			_, err := q.BlockAccount(ctx, id)
			return err
		}},
		{"deletion", func(ctx context.Context, q *store.Queries, id int64) error {
			_, err := q.RequestDeletion(ctx, id)
			return err
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, conn, q, account, grant := oauthFixture(t)
			other := webAccount(t, ctx, conn, "other")
			otherGrant, err := q.UpsertGrant(ctx, store.UpsertGrantParams{AccountID: other, ClientID: "client", Scope: "mcp:read", Resource: "https://example.invalid/mcp"})
			if err != nil {
				t.Fatal(err)
			}
			for i, id := range []int64{grant, otherGrant.ID} {
				chars := []string{"a", "b", "c", "d", "e", "f"}
				oauthCode(t, ctx, q, id, strings.Repeat(chars[2*i], 64), time.Now().Add(time.Hour))
				oauthToken(t, ctx, q, id, strings.Repeat(chars[2*i+1], 64), time.Now().Add(time.Hour))
				consumed := strings.Repeat(chars[4+i], 64)
				oauthCode(t, ctx, q, id, consumed, time.Now().Add(time.Hour))
				if _, err := q.ConsumeCode(ctx, consumed); err != nil {
					t.Fatal(err)
				}
			}
			if err := tc.act(ctx, q, account); err != nil {
				t.Fatal(err)
			}
			for _, check := range []struct {
				id   int64
				want bool
			}{{grant, true}, {otherGrant.ID, false}} {
				var grantRevoked, codeRevoked, tokenRevoked bool
				err := conn.QueryRow(ctx, `SELECT g.revoked_at IS NOT NULL, (SELECT bool_and(c.revoked_at IS NOT NULL) FROM oauth_code c WHERE c.grant_id = g.id), (SELECT bool_and(t.revoked_at IS NOT NULL) FROM oauth_token t WHERE t.grant_id = g.id) FROM oauth_grant g WHERE g.id = $1`, check.id).Scan(&grantRevoked, &codeRevoked, &tokenRevoked)
				if err != nil || grantRevoked != check.want || codeRevoked != check.want || tokenRevoked != check.want {
					t.Fatalf("grant=%v code=%v token=%v want=%v err=%v", grantRevoked, codeRevoked, tokenRevoked, check.want, err)
				}
			}
		})
	}
}
