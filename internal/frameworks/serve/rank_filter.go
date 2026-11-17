package serve

func (e *Engine) rankFiltered(chunks []candidate, controls, allowed map[int64]*control) []ranked {
	global := e.rank(chunks, controls)
	if len(allowed) == len(controls) {
		return global
	}
	result := make([]ranked, 0, len(global))
	seen := map[int64]bool{}
	for _, hit := range global {
		if _, ok := allowed[hit.controlID]; ok {
			result = append(result, hit)
			seen[hit.controlID] = true
		}
	}
	restricted := make([]candidate, 0, len(chunks))
	for _, c := range chunks {
		if _, ok := allowed[c.controlID]; ok {
			restricted = append(restricted, c)
		}
	}
	for _, hit := range e.rank(restricted, allowed) {
		if !seen[hit.controlID] {
			result = append(result, hit)
		}
	}
	return result
}
