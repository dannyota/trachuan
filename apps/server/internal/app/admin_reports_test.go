package app

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/admin"
	"danny.vn/trachuan/apps/server/internal/store"
)

type triageFixture struct{ admin.Store }

func (triageFixture) TriageGroups(context.Context, store.TriageGroupsParams) ([]store.TriageGroupsRow, error) {
	return []store.TriageGroupsRow{
		{DocumentKey: "vn:1", Anchor: "dieu-1", Kind: "broken_citation", Target: "1/2024/ND-CP", ReportCount: 2, AccountCount: 2, Newest: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), Suggestions: []string{"{}"}},
		{DocumentKey: "vn:1", Anchor: "dieu-1", Kind: "broken_citation", Target: "2/2024/ND-CP", ReportCount: 1, AccountCount: 1, Newest: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), Suggestions: []string{"{}"}},
	}, nil
}

func TestTriage_GroupsAndCounts(t *testing.T) {
	var out bytes.Buffer
	if err := runAdmin(context.Background(), []string{"reports", "list"}, triageFixture{}, &out); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected two groups: %s", out.String())
	}
	for i, want := range []struct{ target, reports, accounts string }{{"1/2024/ND-CP", "2", "2"}, {"2/2024/ND-CP", "1", "1"}} {
		fields := strings.Fields(lines[i+1])
		if len(fields) < 9 || fields[4] != want.target || fields[5] != want.reports || fields[6] != want.accounts {
			t.Fatalf("group %d: %s", i, lines[i+1])
		}
	}
}
