package ocrcheck

import (
	"context"
	"encoding/json"
	"testing"
)

func TestCheck_EntriesSelection(t *testing.T) {
	for _, version := range []int{2, 3} {
		t.Run(string(rune('0'+version)), func(t *testing.T) {
			var r *Runner
			var repo *memoryRepository
			var docs []Document
			var o Options
			if version == 2 {
				r, repo, _, docs = fixture(t)
				d := docs[0]
				d.Entry.ID = 2
				docs = append(docs, d)
				o = options()
			} else {
				r, repo, _, docs = concurrentFixture(t, 2)
				o = jpegOptions()
			}
			if err := json.Unmarshal([]byte(`{"Entries":[2,99]}`), &o); err != nil {
				t.Fatal(err)
			}
			s, err := r.Run(context.Background(), docs, o)
			if err != nil || len(repo.attempts) != 1 || repo.attempts[0].EntryID != 2 {
				t.Fatalf("attempts=%+v summary=%+v err=%v", repo.attempts, s, err)
			}
			found := false
			for _, d := range s.Documents {
				if d.EntryID == 99 && d.Outcome == "not_selected" && d.CostMicroUSD == 0 {
					found = true
				}
			}
			if !found {
				t.Fatalf("unknown entry missing from summary: %+v", s)
			}
			s, err = r.Run(context.Background(), docs, o)
			if err != nil || len(repo.attempts) != 1 || len(s.Documents) != 2 {
				t.Fatalf("completed entry retried or not reported: %+v err=%v", s, err)
			}
		})
	}
}

func TestCheck_EntriesPreserveRetryAndDryRun(t *testing.T) {
	for _, version := range []int{2, 3} {
		t.Run(string(rune('0'+version)), func(t *testing.T) {
			var r *Runner
			var repo *memoryRepository
			var docs []Document
			var o Options
			if version == 2 {
				r, repo, _, docs = fixture(t)
				o = options()
			} else {
				r, repo, _, docs = concurrentFixture(t, 1)
				o = jpegOptions()
			}
			o.Entries = []int64{1}
			repo.attempts = []Attempt{
				{EntryID: 1, Checker: o.CheckerID(), State: "failed", Usage: Usage{Input: 1}},
				{EntryID: 1, Checker: o.CheckerID(), State: "failed", Usage: Usage{Input: 1}},
			}
			s, err := r.Run(context.Background(), docs, o)
			if err != nil || len(repo.attempts) != 2 || s.SkippedRetryLimit != 1 || len(s.Documents) != 1 || s.Documents[0].Outcome != "retry_limit" || s.Documents[0].CostMicroUSD != 0 {
				t.Fatalf("summary=%+v attempts=%d err=%v", s, len(repo.attempts), err)
			}
			o.RetryFailed = true
			o.DryRun = true
			s, err = r.Run(context.Background(), docs, o)
			if err != nil || s.Estimated != 1 || len(repo.attempts) != 2 {
				t.Fatalf("dry run summary=%+v attempts=%d err=%v", s, len(repo.attempts), err)
			}
			o.DryRun = false
			s, err = r.Run(context.Background(), docs, o)
			if err != nil || s.Complete != 1 || len(repo.attempts) != 3 {
				t.Fatalf("retry summary=%+v attempts=%d err=%v", s, len(repo.attempts), err)
			}
		})
	}
}

func TestCheck_WithoutEntriesSelectsAll(t *testing.T) {
	r, repo, _, docs := concurrentFixture(t, 2)
	s, err := r.Run(context.Background(), docs, jpegOptions())
	if err != nil || s.Complete != 2 || len(repo.attempts) != 2 {
		t.Fatalf("summary=%+v attempts=%d err=%v", s, len(repo.attempts), err)
	}
}
