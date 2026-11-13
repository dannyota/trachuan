package evaluation

import (
	"context"
	"sync"
	"time"

	"danny.vn/trachuan/internal/retrieval"
)

const caseConcurrency = 3

type orderedPool[T any] struct {
	cancel  context.CancelFunc
	jobs    chan int
	results []chan T
	next    int
	wg      sync.WaitGroup
}

func newOrderedPool[T any](ctx context.Context, count, limit int, work func(context.Context, int) T) *orderedPool[T] {
	ctx, cancel := context.WithCancel(ctx)
	p := &orderedPool[T]{cancel: cancel, jobs: make(chan int, 2*limit), results: make([]chan T, count)}
	for i := range p.results {
		p.results[i] = make(chan T, 1)
	}
	for i := 0; i < limit; i++ {
		p.wg.Add(1)
		go func() {
			defer p.wg.Done()
			for index := range p.jobs {
				p.results[index] <- work(ctx, index)
			}
		}()
	}
	for p.next < count && p.next < limit {
		p.jobs <- p.next
		p.next++
	}
	return p
}

func (p *orderedPool[T]) take(index int) T {
	result := <-p.results[index]
	if p.next < len(p.results) {
		p.jobs <- p.next
		p.next++
	}
	return result
}

func (p *orderedPool[T]) stop() {
	p.cancel()
	close(p.jobs)
	p.wg.Wait()
}

type caseCollect struct {
	arms    retrieval.Arms
	err     error
	elapsed float64
}

type casePrefetch struct {
	product, legacy       retrieval.Prepared
	prepareErr, legacyErr error
	collects              map[string]map[retrieval.Mode]caseCollect
}

func prefetchCase(ctx context.Context, engine Engine, input cohortInput, rejects map[string]bool, modes []retrieval.Mode, recipe retrieval.Recipe, policy ValidityOverrides) casePrefetch {
	c := input.c
	force := inForceForCase(c, policy)
	request := retrieval.Request{Query: c.Question, Limit: 8, Mode: "exact", Arms: "hybrid", InForceOnly: &force}
	result := casePrefetch{collects: map[string]map[retrieval.Mode]caseCollect{}}
	result.product, result.prepareErr = engine.Prepare(ctx, request)
	if result.prepareErr != nil {
		return result
	}
	result.legacy = result.product
	if input.legacy && !result.product.Validation.Accepted {
		result.legacy, result.legacyErr = engine.PrepareLegacy(ctx, request)
		if result.legacyErr != nil {
			return result
		}
	}
	if input.legacy && (!result.legacy.Validation.Accepted || result.legacy.Vector == nil) {
		return result
	}
	if (!input.legacy || !rejects[c.ID]) && (!result.product.Validation.Accepted || result.product.Vector == nil) {
		return result
	}
	for _, cohort := range input.cohorts {
		if cohort == "old_product" && rejects[c.ID] {
			continue
		}
		p := result.product
		if cohort == "legacy" {
			p = result.legacy
		}
		result.collects[cohort] = map[retrieval.Mode]caseCollect{}
		var exact *retrieval.LexicalArms
		for _, mode := range modes {
			p.Request.Mode = mode
			depths := retrieval.Depths{Dense: recipe.DenseDepth, Lexical: recipe.LexicalDepth}
			if mode == "exact" {
				depths = retrieval.Depths{Dense: 200, Lexical: 100}
			}
			started := time.Now()
			var source *retrieval.LexicalArms
			if mode == "ann" {
				source = exact
			}
			arms, err := collectEvaluation(ctx, engine, p, depths, force, policy, source)
			if mode == "exact" && err == nil {
				exact = &retrieval.LexicalArms{Primary: arms.Primary.Lexical, NonCurrent: arms.NonCurrent.Lexical, Depth: arms.Depths.Lexical}
			}
			result.collects[cohort][mode] = caseCollect{arms: arms, err: err, elapsed: float64(time.Since(started)) / float64(time.Millisecond)}
			if err != nil {
				return result
			}
		}
	}
	return result
}

type calibrationPrefetch struct {
	prepared               retrieval.Prepared
	arms                   retrieval.Arms
	prepareErr, collectErr error
}

func prefetchCalibration(ctx context.Context, engine Engine, c Case, depths retrieval.Depths) calibrationPrefetch {
	force := true
	var result calibrationPrefetch
	result.prepared, result.prepareErr = engine.Prepare(ctx, retrieval.Request{Query: c.Question, Limit: 8, Mode: "exact", Arms: "hybrid", InForceOnly: &force})
	if result.prepareErr == nil && result.prepared.Validation.Accepted {
		result.arms, result.collectErr = engine.Collect(ctx, result.prepared, depths)
	}
	return result
}
