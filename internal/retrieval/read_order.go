package retrieval

import "context"

type nameBucket struct{ start, end, depth int }

func (tree *readTree) orderChildren(ctx context.Context, children []int) ([]int, error) {
	ordered := append([]int(nil), children...)
	sorted := true
	for i := 1; i < len(ordered); i++ {
		if i%128 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		a, b := tree.nodes[ordered[i-1]].provision, tree.nodes[ordered[i]].provision
		if a.Ordinal > b.Ordinal || a.Ordinal == b.Ordinal && a.NodeID > b.NodeID {
			sorted = false
			break
		}
	}
	if sorted {
		return ordered, ctx.Err()
	}
	// Stable byte radix sorting keeps sibling order identical to renderTree.
	temp := make([]int, len(ordered))
	stack := []nameBucket{{0, len(ordered), 0}}
	for len(stack) > 0 {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		group := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		var counts [257]int
		for i := group.start; i < group.end; i++ {
			if i%128 == 0 {
				if err := ctx.Err(); err != nil {
					return nil, err
				}
			}
			name := tree.nodes[ordered[i]].provision.NodeID
			b := 0
			if group.depth < len(name) {
				b = int(name[group.depth]) + 1
			}
			counts[b]++
		}
		var start, next [257]int
		pos := group.start
		for b, n := range counts {
			start[b] = pos
			next[b] = pos
			pos += n
		}
		for i := group.start; i < group.end; i++ {
			if i%128 == 0 {
				if err := ctx.Err(); err != nil {
					return nil, err
				}
			}
			name := tree.nodes[ordered[i]].provision.NodeID
			b := 0
			if group.depth < len(name) {
				b = int(name[group.depth]) + 1
			}
			temp[next[b]] = ordered[i]
			next[b]++
		}
		copy(ordered[group.start:group.end], temp[group.start:group.end])
		for b := 1; b < len(counts); b++ {
			if counts[b] > 1 {
				stack = append(stack, nameBucket{start[b], start[b] + counts[b], group.depth + 1})
			}
		}
	}
	for shift := uint(0); shift < 64; shift += 8 {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		var counts [256]int
		for i, index := range ordered {
			if i%128 == 0 {
				if err := ctx.Err(); err != nil {
					return nil, err
				}
			}
			key := uint64(int64(tree.nodes[index].provision.Ordinal)) ^ (uint64(1) << 63)
			counts[byte(key>>shift)]++
		}
		total := 0
		for i, n := range counts {
			counts[i] = total
			total += n
		}
		for i, index := range ordered {
			if i%128 == 0 {
				if err := ctx.Err(); err != nil {
					return nil, err
				}
			}
			key := uint64(int64(tree.nodes[index].provision.Ordinal)) ^ (uint64(1) << 63)
			b := byte(key >> shift)
			temp[counts[b]] = index
			counts[b]++
		}
		ordered, temp = temp, ordered
	}
	return ordered, nil
}
