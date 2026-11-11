package retrieval

import "context"

type readMatch struct {
	count int
	first int
	end   int
}

type matchNode struct {
	firstByte   byte
	firstChild  int
	edges       map[byte]int
	transitions map[byte]int
	fail        int
	output      int
	terminal    int
	jump        int
}

type readMatcher struct {
	nodes     []matchNode
	patterns  []string
	matches   []readMatch
	byPattern map[string]int
}

func newReadMatcher() *readMatcher {
	return &readMatcher{nodes: []matchNode{{output: -1, terminal: -1, jump: -1}}, byPattern: make(map[string]int)}
}

func (m *readMatcher) add(ctx context.Context, pattern string) (int, error) {
	if id, ok := m.byPattern[pattern]; ok {
		return id, nil
	}
	id := len(m.patterns)
	m.patterns = append(m.patterns, pattern)
	m.matches = append(m.matches, readMatch{first: -1})
	m.byPattern[pattern] = id
	current := 0
	for i := 0; i < len(pattern); i++ {
		if i%1024 == 0 {
			if err := ctx.Err(); err != nil {
				return 0, err
			}
		}
		next, ok := m.edge(current, pattern[i])
		if !ok {
			next = len(m.nodes)
			if m.nodes[current].firstChild == 0 {
				m.nodes[current].firstByte = pattern[i]
				m.nodes[current].firstChild = next
			} else {
				if m.nodes[current].edges == nil {
					m.nodes[current].edges = make(map[byte]int)
				}
				m.nodes[current].edges[pattern[i]] = next
			}
			m.nodes = append(m.nodes, matchNode{output: -1, terminal: -1, jump: -1})
		}
		current = next
	}
	m.nodes[current].terminal = id
	return id, nil
}

func (m *readMatcher) edge(node int, b byte) (int, bool) {
	n := &m.nodes[node]
	if n.firstChild != 0 && n.firstByte == b {
		return n.firstChild, true
	}
	child, ok := n.edges[b]
	return child, ok
}

func (m *readMatcher) transition(ctx context.Context, node int, b byte) (int, error) {
	var path []int
	current := node
	for {
		if len(path) > 0 && len(path)%128 == 0 {
			if err := ctx.Err(); err != nil {
				return 0, err
			}
		}
		if next, ok := m.edge(current, b); ok {
			current = next
			break
		}
		if next, ok := m.nodes[current].transitions[b]; ok {
			current = next
			break
		}
		if current == 0 {
			break
		}
		path = append(path, current)
		current = m.nodes[current].fail
	}
	for offset, i := range path {
		if offset%128 == 0 {
			if err := ctx.Err(); err != nil {
				return 0, err
			}
		}
		if m.nodes[i].transitions == nil {
			m.nodes[i].transitions = make(map[byte]int)
		}
		m.nodes[i].transitions[b] = current
	}
	return current, nil
}

func (m *readMatcher) prepare(ctx context.Context) error {
	queue := make([]int, 0, len(m.nodes))
	if m.nodes[0].firstChild != 0 {
		queue = append(queue, m.nodes[0].firstChild)
	}
	for _, child := range m.nodes[0].edges {
		queue = append(queue, child)
	}
	for i := 0; i < len(queue); i++ {
		if i%128 == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		node := queue[i]
		addChild := func(b byte, child int) error {
			failure, err := m.transition(ctx, m.nodes[node].fail, b)
			if err != nil {
				return err
			}
			m.nodes[child].fail = failure
			if m.nodes[failure].terminal >= 0 {
				m.nodes[child].output = failure
			} else {
				m.nodes[child].output = m.nodes[failure].output
			}
			queue = append(queue, child)
			return nil
		}
		if m.nodes[node].firstChild != 0 {
			if err := addChild(m.nodes[node].firstByte, m.nodes[node].firstChild); err != nil {
				return err
			}
		}
		for b, child := range m.nodes[node].edges {
			if err := addChild(b, child); err != nil {
				return err
			}
		}
	}
	return nil
}

// Completed patterns link to the next active suffix so scans skip repeat matches.
func (m *readMatcher) available(node int) int {
	if node < 0 {
		return -1
	}
	n := &m.nodes[node]
	if n.jump < 0 {
		return node
	}
	n.jump = m.available(n.jump-1) + 1
	return n.jump - 1
}

func (m *readMatcher) scan(ctx context.Context, text string) error {
	if err := m.prepare(ctx); err != nil {
		return err
	}
	state := 0
	for i := 0; i < len(text); i++ {
		if i%1024 == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		var err error
		state, err = m.transition(ctx, state, text[i])
		if err != nil {
			return err
		}
		node := state
		if m.nodes[node].terminal < 0 {
			node = m.nodes[node].output
		}
		steps := 0
		for node = m.available(node); node >= 0; node = m.available(m.nodes[node].output) {
			if steps%128 == 0 {
				if err := ctx.Err(); err != nil {
					return err
				}
			}
			steps++
			id := m.nodes[node].terminal
			start := i + 1 - len(m.patterns[id])
			match := &m.matches[id]
			if match.count == 0 {
				match.count = 1
				match.first = start
				match.end = i + 1
			} else if start >= match.end {
				match.count = 2
				m.nodes[node].jump = m.available(m.nodes[node].output) + 1
			}
		}
	}
	return ctx.Err()
}
