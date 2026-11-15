package relate

import (
	"bufio"
	"fmt"
	"io"
	"sort"

	"danny.vn/trachuan/internal/frameworks/registry"
)

func Report(w io.Writer, edges []Edge, stats Stats, reg *registry.Registry) error {
	sources, relations, targets := map[string]int{}, map[string]int{}, map[string]int{}
	resolved := 0
	for _, e := range edges {
		sources[e.Source]++
		relations[e.Relation]++
		if e.Resolved {
			resolved++
		}
		target := "other"
		for _, v := range reg.Frameworks {
			if v.Code == e.To.Code && v.Version == e.To.Version {
				target = v.Code + ":" + v.Version
				break
			}
		}
		targets[target]++
	}
	out := bufio.NewWriter(w)
	_, _ = fmt.Fprintf(out, "resolved: %d\nunresolved: %d\ndropped: %d\nmerged duplicates: %d\n", resolved, stats.Unresolved, stats.Dropped, stats.Duplicates)
	for _, group := range []struct {
		name   string
		values map[string]int
	}{{"source", sources}, {"relation", relations}, {"target", targets}} {
		keys := make([]string, 0, len(group.values))
		for k := range group.values {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			_, _ = fmt.Fprintf(out, "%s %s: %d\n", group.name, k, group.values[k])
		}
	}
	if err := out.Flush(); err != nil {
		return fmt.Errorf("write statistics: %w", err)
	}
	return nil
}
