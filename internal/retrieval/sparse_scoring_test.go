package retrieval

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"danny.vn/trachuan/internal/retrieval/lexical"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestSparseScoring_ResetsPoolAndHonorsScope(t *testing.T) {
	date := time.Date(2099, 1, 2, 0, 0, 0, 0, time.UTC)
	index := &sparsePostings{
		dimensions: []sparseDimension{{index: 1, start: 0, end: 2}, {index: 4, start: 2, end: 3}},
		entries:    []sparsePosting{{position: 0, weight: 2}, {position: 1, weight: 3}, {position: 0, weight: 1}},
		chunks:     []sparseChunk{{id: 10, document: 0}, {id: 20, document: 1}},
		documents:  []sparseDocument{{current: true, class: "primary"}, {current: true, class: "relation_context", number: "09/X"}},
	}
	for _, tc := range []struct {
		query lexical.Vector
		refs  []string
		want  [2][]sparseScored
	}{
		{query: lexical.Vector{{Index: 1, Value: 1}, {Index: 4, Value: 1}}, refs: []string{"09/X"}, want: [2][]sparseScored{{{id: 10, score: 3}, {id: 20, score: 3}}, nil}},
		{query: lexical.Vector{{Index: 4, Value: 1}}, want: [2][]sparseScored{{{id: 10, score: 1}}, nil}},
		{query: lexical.Vector{{Index: 1, Value: 1}}, refs: []string{"9/X"}, want: [2][]sparseScored{{{id: 10, score: 2}}, nil}},
		{query: lexical.Vector{{Index: 2, Value: 1}}},
	} {
		got, err := index.candidates(t.Context(), tc.query, date, false, false, tc.refs, 1)
		if err != nil || !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("candidates=%v err=%v want=%v", got, err, tc.want)
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := index.candidates(ctx, lexical.Vector{{Index: 1, Value: 1}}, date, false, false, nil, 1); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: %v", err)
	}
}

func TestSparseEligibility_DatesConflictAndTrial(t *testing.T) {
	date := time.Date(2099, 1, 2, 0, 0, 0, 0, time.UTC)
	bound := pgtype.Date{Time: date, Valid: true}
	for _, tc := range []struct {
		name                    string
		doc                     sparseDocument
		historical, trial, want bool
	}{
		{name: "null bounds", doc: sparseDocument{current: true}, want: true},
		{name: "inclusive start", doc: sparseDocument{current: true, from: bound}, want: true},
		{name: "exclusive end", doc: sparseDocument{current: true, to: bound}},
		{name: "conflict", doc: sparseDocument{current: true, conflict: true}},
		{name: "historical conflict", doc: sparseDocument{current: true, conflict: true}, historical: true, want: true},
		{name: "not yet", doc: sparseDocument{status: "not_yet", from: pgtype.Date{Time: date.AddDate(1, 0, 0), Valid: true}}, trial: true, want: true},
		{name: "historical excludes trial", doc: sparseDocument{status: "not_yet", from: pgtype.Date{Time: date.AddDate(1, 0, 0), Valid: true}}, historical: true, trial: true},
		{name: "infinite open interval", doc: sparseDocument{current: true, from: pgtype.Date{Valid: true, InfinityModifier: pgtype.NegativeInfinity}, to: pgtype.Date{Valid: true, InfinityModifier: pgtype.Infinity}}, want: true},
		{name: "positive infinite start", doc: sparseDocument{current: true, from: pgtype.Date{Valid: true, InfinityModifier: pgtype.Infinity}}},
		{name: "negative infinite end", doc: sparseDocument{current: true, to: pgtype.Date{Valid: true, InfinityModifier: pgtype.NegativeInfinity}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.doc.eligible(date, tc.historical, tc.trial); got != tc.want {
				t.Fatalf("eligible=%t want=%t", got, tc.want)
			}
		})
	}
}

func TestSparseSize_RejectsOverBudgetBeforeAllocation(t *testing.T) {
	steady, peak, ok := (sparseSize{chunks: 221114, postings: 16203127, documents: 3842}).estimate(1048576)
	if !ok || steady < 130_000_000 || peak > 600_000_000 {
		t.Fatalf("release estimate: steady=%d peak=%d allowed=%t", steady, peak, ok)
	}
	t.Logf("estimated steady=%d peak including retiring index=%d", steady, peak)
	if _, _, ok := (sparseSize{postings: 30_000_000}).estimate(1048576); ok {
		t.Fatal("accepted oversized build")
	}
}
