package retrieval

import (
	"context"
	"math"
	"math/rand/v2"
	"slices"
	"testing"

	"danny.vn/trachuan/internal/retrieval/lexical"
)

func TestSparseDecode_StrictFloat32(t *testing.T) {
	got, bound, err := decodeSparse("{1:0.1,4:2.0000002}/8", 8)
	if err != nil || len(got) != 2 || got[0].Index != 1 || got[0].Value != float32(.1) || got[1].Value != math.Nextafter32(2, 3) || bound <= 0 {
		t.Fatalf("decode: %v %g %v", got, bound, err)
	}
	for _, text := range []string{"{2:1,1:1}/8", "{1:1,1:2}/8", "{0:1}/8", "{9:1}/8", "{1:0}/8", "{1:-1}/8", "{1:NaN}/8", "{1:Inf}/8", "{1:1e40}/8", "{1:1.7014117e38,2:1.7014117e38}/8", "{1:1e-50}/8", "{1:1}/9", "{1:1,}/8", "{1}/8", "1:1/8"} {
		if _, _, err := decodeSparse(text, 8); err == nil {
			t.Errorf("accepted %q", text)
		}
	}
	if got, bound, err := decodeSparse("{}/8", 8); err != nil || len(got) != 0 || bound != 0 {
		t.Fatalf("empty: %v %g %v", got, bound, err)
	}
}

func TestSparseDecode_AccountsForEveryAddition(t *testing.T) {
	_, bound, err := decodeSparse("{1:1,2:1}/8", 8)
	if err != nil || math.Abs(bound-2.408027792455422e-7) > 1e-20 {
		t.Fatalf("bound=%g err=%v", bound, err)
	}
	_, bound, err = decodeSparse("{1:2}/8", 8)
	if err != nil || bound != 0 {
		t.Fatalf("single-term bound=%g err=%v", bound, err)
	}
}

func TestSparseCandidates_ContainsSQLBoundaryTies(t *testing.T) {
	rng := rand.New(rand.NewPCG(19, 27))
	const delta = .125
	for trial := 0; trial < 300; trial++ {
		memory := make([]sparseScored, 80)
		sql := make([]sparseScored, 80)
		for i := range memory {
			score := float64(rng.IntN(12) + 1)
			memory[i] = sparseScored{id: int64(i + 1), score: score + (rng.Float64()*2-1)*delta}
			sql[i] = sparseScored{id: int64(i + 1), score: score}
		}
		slices.SortFunc(sql, sparseCandidateOrder)
		for _, depth := range []int{1, 10, 79, 100} {
			got := sparseCandidates(slices.Clone(memory), depth, delta)
			ids := map[int64]bool{}
			for _, c := range got {
				ids[c.id] = true
			}
			boundary := sql[min(depth, len(sql))-1].score
			for _, c := range sql {
				if c.score >= boundary && !ids[c.id] {
					t.Fatalf("trial %d depth %d omitted boundary candidate %d", trial, depth, c.id)
				}
			}
		}
	}
	got := sparseCandidates([]sparseScored{{id: 1, score: 10}, {id: 2, score: 9.75}, {id: 3, score: 9.749}}, 1, delta)
	if len(got) != 2 {
		t.Fatalf("inclusive boundary: %v", got)
	}
}

func TestSparseQuery_NonUnitWeightsFallBack(t *testing.T) {
	e := &Engine{}
	e.sparse.index.Store(&sparsePostings{})
	for _, weight := range []float32{0, -1, 2, math.Nextafter32(1, 2), float32(math.NaN())} {
		if e.postingsFor(lexical.Vector{{Index: 1, Value: weight}}) != nil {
			t.Fatalf("postings accepted weight %g", weight)
		}
	}
	if e.postingsFor(lexical.Vector{{Index: 1, Value: 1}}) == nil {
		t.Fatal("unit query did not use published postings")
	}
}

func TestSparseLifecycle_CloseAndRetireBlockPublication(t *testing.T) {
	for _, retire := range []bool{false, true} {
		e := &Engine{}
		e.sparse.init()
		if retire {
			e.Retire()
		} else {
			e.Close()
		}
		if e.publishPostings(&sparsePostings{}) {
			t.Fatal("published after close or retirement")
		}
		if e.sparse.ctx.Err() != context.Canceled {
			t.Fatal("loader context not canceled")
		}
		e.Close()
	}
}

func TestSparseLifecycle_CloseWaitsForLoader(t *testing.T) {
	e := &Engine{}
	e.sparse.init()
	entered, release, finished := make(chan struct{}), make(chan struct{}), make(chan struct{})
	e.sparse.wg.Add(1)
	go func() { defer e.sparse.wg.Done(); <-e.sparse.ctx.Done(); close(entered); <-release }()
	go func() { e.Close(); close(finished) }()
	<-entered
	select {
	case <-finished:
		t.Fatal("Close returned before loader")
	default:
	}
	close(release)
	<-finished
}
