package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/server/internal/store"
)

type showIDsFixture struct {
	reportsFixture
	rows []store.IssueReport
}

func (s *showIDsFixture) reportsByIDs(_ context.Context, ids []int64) ([]store.IssueReport, error) {
	var found []store.IssueReport
	for _, row := range s.rows {
		for _, id := range ids {
			if id == row.ID {
				found = append(found, row)
				break
			}
		}
	}
	return found, nil
}
func TestTriage_ShowByIDs(t *testing.T) {
	s := &showIDsFixture{rows: []store.IssueReport{{ID: 12, AccountID: 987654, DocumentKey: "vn:testdoc", Anchor: "dieu-1", Kind: "broken_citation", Target: "2/2024/QH15", Release: "20990101", State: "open", Suggestion: json.RawMessage(`{}`)}, {ID: 13, AccountID: 456789, Release: "20990102", State: "rejected", Suggestion: json.RawMessage(`{}`)}, {ID: 14, AccountID: 111111, Release: "20990103", State: "open", Suggestion: json.RawMessage(`{}`)}}}
	for _, tc := range []struct {
		name     string
		args     []string
		accounts bool
	}{{"without accounts", []string{"show", "--ids", "12,13"}, false}, {"with accounts", []string{"show", "--ids", "12", "13", "--accounts"}, true}, {"accounts first", []string{"show", "--accounts", "--ids", "12", "13"}, true}} {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			err := runAdmin(context.Background(), append([]string{"reports"}, tc.args...), s, &out)
			if err != nil {
				t.Fatal(err)
			}
			text := out.String()
			for _, value := range []string{"12", "13", "20990101", "20990102", "rejected", "vn:testdoc", "dieu-1", "broken_citation", "2/2024/QH15"} {
				if !strings.Contains(text, value) {
					t.Errorf("missing %s: %s", value, text)
				}
			}
			for _, value := range []string{"20990103", "111111", "email", "google_sub", "subject"} {
				if strings.Contains(text, value) {
					t.Errorf("unexpected %s", value)
				}
			}
			for _, value := range []string{"987654", "456789"} {
				if strings.Contains(text, value) != tc.accounts {
					t.Errorf("account visibility %s: %s", value, text)
				}
			}
		})
	}
	for _, args := range [][]string{{"show", "--ids", "12", "99"}, {"show", "--ids"}, {"show", "--ids", "0"}, {"show", "--ids", "12", "bad"}} {
		t.Run(fmt.Sprint(args), func(t *testing.T) {
			var out bytes.Buffer
			err := runAdmin(context.Background(), append([]string{"reports"}, args...), s, &out)
			if err == nil || out.Len() != 0 {
				t.Fatalf("error=%v output=%q", err, out.String())
			}
		})
	}
}
