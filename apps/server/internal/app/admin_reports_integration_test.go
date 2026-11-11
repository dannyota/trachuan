//go:build integration

package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/store"
)

func TestExport_DatabaseMembershipAndRecovery(t *testing.T) {
	ctx, pool, s, account := boundsFixture(t)
	insert := func(key string) int64 {
		t.Helper()
		r, err := s.InsertReport(ctx, store.InsertReportParams{AccountID: account, CreatedAt: time.Now(), Release: "20990101", DocumentKey: key, DocumentNumber: "1/2024/QH15", Kind: "wrong_validity", Suggestion: json.RawMessage(`{"status_class":"in_force"}`)})
		if err != nil {
			t.Fatal(err)
		}
		_, err = pool.Exec(ctx, "UPDATE issue_report SET state='accepted' WHERE id=$1", r.ID)
		if err != nil {
			t.Fatal(err)
		}
		return r.ID
	}
	first := insert("vn:one")
	dir := t.TempDir()
	var out bytes.Buffer
	if err := runAdmin(ctx, []string{"reports", "export", "--out", dir}, s, &out); err != nil {
		t.Fatal(err)
	}
	id := strings.TrimSpace(out.String())
	second := insert("vn:two")
	other := t.TempDir()
	if err := runAdmin(ctx, []string{"reports", "export", "--out", other, "--batch", id}, s, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	manifest := func(root, batch string) []int64 {
		t.Helper()
		b, err := os.ReadFile(filepath.Join(root, batch, "manifest.json"))
		if err != nil {
			t.Fatal(err)
		}
		var v struct {
			IDs []int64 `json:"report_ids"`
		}
		if err = json.Unmarshal(b, &v); err != nil {
			t.Fatal(err)
		}
		return v.IDs
	}
	ids := manifest(other, id)
	if len(ids) != 1 || ids[0] != first {
		t.Fatalf("recovery membership %v", ids)
	}
	out.Reset()
	if err := runAdmin(ctx, []string{"reports", "export", "--out", dir}, s, &out); err != nil {
		t.Fatal(err)
	}
	ids = manifest(dir, strings.TrimSpace(out.String()))
	if len(ids) != 1 || ids[0] != second {
		t.Fatalf("new membership %v", ids)
	}
	if _, err := pool.Exec(ctx, "DELETE FROM account WHERE id=$1", account); err != nil {
		t.Fatal(err)
	}
	snapshot, err := s.reportsSnapshot(context.Background(), id)
	if err != nil || !bytes.Contains(snapshot.Reports, []byte(`"report_id"`)) {
		t.Fatalf("snapshot unavailable after account deletion: %v", err)
	}
}

func TestExport_RetainedCapRollsBack(t *testing.T) {
	ctx, pool, s, account := boundsFixture(t)
	_, err := s.InsertReport(ctx, store.InsertReportParams{AccountID: account, CreatedAt: time.Now(), Release: "20990101", DocumentKey: "vn:one", DocumentNumber: "1/2024/QH15", Kind: "wrong_validity", Suggestion: json.RawMessage(`{}`)})
	if err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, `UPDATE issue_report SET state='accepted'; INSERT INTO issue_report_export (batch_id,report_id,document_key,anchor,document_number,release,kind,target,suggestion) SELECT 'old',i,'vn:old','','','20990101','wrong_text','','{}'::jsonb FROM generate_series(1,99999) i`)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.createReportsBatch(ctx, "new"); err == nil || err.Error() != "reports retained cap reached" {
		t.Fatalf("cap: %v", err)
	}
	var count int
	if err = pool.QueryRow(ctx, "SELECT count(*) FROM issue_report WHERE export_batch IS NOT NULL").Scan(&count); err != nil || count != 0 {
		t.Fatalf("partial batch: %d %v", count, err)
	}
}

func TestDecide_DatabaseUnknownIDRollsBack(t *testing.T) {
	ctx, pool, s, account := boundsFixture(t)
	r, err := s.InsertReport(ctx, store.InsertReportParams{AccountID: account, CreatedAt: time.Now(), Release: "20990101", DocumentKey: "vn:one", Kind: "wrong_text", Suggestion: json.RawMessage(`{}`)})
	if err != nil {
		t.Fatal(err)
	}
	err = runAdmin(ctx, []string{"reports", "accept", "--ids", fmt.Sprint(r.ID, ",", r.ID+1), "--by", "operator"}, s, &bytes.Buffer{})
	if err == nil {
		t.Fatal("unknown ID accepted")
	}
	var state string
	if err = pool.QueryRow(ctx, "SELECT state FROM issue_report WHERE id=$1", r.ID).Scan(&state); err != nil || state != "open" {
		t.Fatalf("state %q: %v", state, err)
	}
}

func TestDecide_DatabaseSelectors(t *testing.T) {
	for _, mode := range []string{"group", "ids"} {
		t.Run(mode, func(t *testing.T) {
			ctx, pool, s, account := boundsFixture(t)
			other, err := s.CreateAccount(ctx, store.CreateAccountParams{GoogleSub: "selector-other", Email: "selector@example.invalid"})
			if err != nil {
				t.Fatal(err)
			}
			insert := func(accountID int64, key, state string) int64 {
				t.Helper()
				r, err := s.InsertReport(ctx, store.InsertReportParams{AccountID: accountID, CreatedAt: time.Now(), Release: "20990101", DocumentKey: key, Kind: "wrong_text", Suggestion: json.RawMessage(`{}`)})
				if err != nil {
					t.Fatal(err)
				}
				if state != "open" {
					if _, err = pool.Exec(ctx, "UPDATE issue_report SET state=$1 WHERE id=$2", state, r.ID); err != nil {
						t.Fatal(err)
					}
				}
				return r.ID
			}
			selectedOpen := insert(account, "vn:selected", "open")
			selectedClosed := insert(other.ID, "vn:selected", "accepted")
			otherOpen := insert(account, "vn:other", "open")
			otherClosed := insert(other.ID, "vn:other", "rejected")
			var args []string
			wantSelected := "accepted"
			if mode == "group" {
				wantSelected = "rejected"
				id := groupID(store.TriageGroupsRow{DocumentKey: "vn:selected", Kind: "wrong_text"})
				args = []string{"reports", "reject", id[:12], "--by", "operator"}
			} else {
				args = []string{"reports", "accept", "--ids", fmt.Sprintf("%d,%d", selectedOpen, otherClosed), "--by", "operator"}
			}
			if err = runAdmin(ctx, args, s, &bytes.Buffer{}); err != nil {
				t.Fatal(err)
			}
			for id, want := range map[int64]string{selectedOpen: wantSelected, selectedClosed: "accepted", otherOpen: "open", otherClosed: "rejected"} {
				var state string
				var by *string
				if err = pool.QueryRow(ctx, "SELECT state, decided_by FROM issue_report WHERE id=$1", id).Scan(&state, &by); err != nil {
					t.Fatal(err)
				}
				if state != want {
					t.Errorf("report %d state = %q, want %q", id, state, want)
				}
				if id == selectedOpen {
					if by == nil || *by != "operator" {
						t.Errorf("report %d operator not recorded", id)
					}
				} else if by != nil {
					t.Errorf("unselected or closed report %d was changed", id)
				}
			}
		})
	}
}
