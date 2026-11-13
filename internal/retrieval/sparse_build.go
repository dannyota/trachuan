package retrieval

import "context"

func sortSparseTriples(ctx context.Context, triples []sparseTriple) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	for root := len(triples)/2 - 1; root >= 0; root-- {
		if root%1024 == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		siftSparseTriples(triples, root, len(triples))
	}
	for end := len(triples) - 1; end > 0; end-- {
		if end%1024 == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		triples[0], triples[end] = triples[end], triples[0]
		siftSparseTriples(triples, 0, end)
	}
	return ctx.Err()
}

func siftSparseTriples(triples []sparseTriple, root, end int) {
	for child := root*2 + 1; child < end; child = root*2 + 1 {
		if child+1 < end && sparseTripleLess(triples[child], triples[child+1]) {
			child++
		}
		if !sparseTripleLess(triples[root], triples[child]) {
			return
		}
		triples[root], triples[child] = triples[child], triples[root]
		root = child
	}
}

func sparseTripleLess(a, b sparseTriple) bool {
	return a.dimension < b.dimension || (a.dimension == b.dimension && a.position < b.position)
}

func (p *sparsePostings) buildDirectory(ctx context.Context, triples []sparseTriple) error {
	dimensions := 0
	for i, triple := range triples {
		if i%4096 == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		if i == 0 || triple.dimension != triples[i-1].dimension {
			dimensions++
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	p.dimensions = make([]sparseDimension, 0, dimensions)
	p.entries = make([]sparsePosting, len(triples))
	for i, triple := range triples {
		if i%4096 == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		if i == 0 || triple.dimension != triples[i-1].dimension {
			p.dimensions = append(p.dimensions, sparseDimension{index: triple.dimension, start: uint32(i)})
		}
		p.dimensions[len(p.dimensions)-1].end = uint32(i + 1)
		p.entries[i] = sparsePosting{position: triple.position, weight: triple.weight}
	}
	return ctx.Err()
}
