//go:build integration

package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/server/internal/store"
)

func TestTriage_ShowIDsDatabase(t *testing.T) {
	ctx, pool, s, account := boundsFixture(t)
	insert := func(key string) store.IssueReport {
		t.Helper()
		row, err := s.InsertReport(ctx, store.InsertReportParams{AccountID: account, Release: "20990101", DocumentKey: key, Kind: "wrong_text", Suggestion: json.RawMessage(`{}`)})
		if err != nil {
			t.Fatal(err)
		}
		return row
	}
	first, second := insert("vn:first"), insert("vn:second")
	if _, err := pool.Exec(ctx, "UPDATE issue_report SET state='rejected' WHERE id=$1", second.ID); err != nil {
		t.Fatal(err)
	}
	for _, accounts := range []bool{false, true} {
		args := []string{"reports", "show", "--ids", fmt.Sprint(first.ID), fmt.Sprint(second.ID)}
		if accounts {
			args = append(args, "--accounts")
		}
		var out bytes.Buffer
		if err := runAdmin(ctx, args, s, &out); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out.String(), "rejected") || !strings.Contains(out.String(), "vn:first") || !strings.Contains(out.String(), "vn:second") || strings.Contains(out.String(), "ACCOUNT") != accounts {
			t.Fatalf("output %q", out.String())
		}
	}
	var out bytes.Buffer
	err := runAdmin(ctx, []string{"reports", "show", "--ids", fmt.Sprint(first.ID), fmt.Sprint(second.ID + 1)}, s, &out)
	if err == nil || out.Len() != 0 {
		t.Fatalf("unknown ID: %v output %q", err, out.String())
	}
	var count int
	if err = pool.QueryRow(ctx, "SELECT count(*) FROM issue_report WHERE state='open'").Scan(&count); err != nil || count != 1 {
		t.Fatalf("state changed: %d %v", count, err)
	}
}
