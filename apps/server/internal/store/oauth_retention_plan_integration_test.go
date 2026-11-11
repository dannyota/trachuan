//go:build integration

package store_test

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestStore_OAuthRetentionIndexPlans(t *testing.T) {
	ctx, conn, _ := setup(t)
	if _, err := conn.Exec(ctx, `SET enable_seqscan = off`); err != nil {
		t.Fatal(err)
	}
	src, err := os.ReadFile("../../sql/oauth_retention.sql")
	if err != nil {
		t.Fatal(err)
	}
	period := pgtype.Interval{Microseconds: int64(24 * time.Hour / time.Microsecond), Valid: true}
	for _, tc := range []struct {
		name    string
		indexes []string
	}{
		{"DeleteExpiredOAuthRequests", []string{"oauth_request_expires_at_idx"}},
		{"DeleteExpiredOAuthCodes", []string{"oauth_code_expires_at_idx"}},
		{"DeleteExpiredOAuthTokens", []string{"oauth_token_family_expires_at_idx", "oauth_grant_revoked_at_idx", "oauth_token_grant_id_idx"}},
		{"DeleteRetiredOAuthGrants", []string{"oauth_grant_retirement_anchor_idx"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			part := strings.SplitN(string(src), "-- name: "+tc.name+" :execrows\n", 2)
			if len(part) != 2 {
				t.Fatal("query missing")
			}
			query := strings.SplitN(part[1], "-- name:", 2)[0]
			query = strings.NewReplacer("sqlc.arg(as_of)", "$1", "sqlc.arg(retention)", "$2", "sqlc.arg(batch_limit)", "$3").Replace(query)
			rows, err := conn.Query(ctx, "EXPLAIN (COSTS OFF) "+query, time.Now(), period, int32(10))
			if err != nil {
				t.Fatal(err)
			}
			defer rows.Close()
			var plan strings.Builder
			for rows.Next() {
				var line string
				if err := rows.Scan(&line); err != nil {
					t.Fatal(err)
				}
				plan.WriteString(line)
				plan.WriteByte('\n')
			}
			if err := rows.Err(); err != nil {
				t.Fatal(err)
			}
			for _, index := range tc.indexes {
				if !strings.Contains(plan.String(), index) {
					t.Errorf("plan does not use %s:\n%s", index, plan.String())
				}
			}
		})
	}
}
