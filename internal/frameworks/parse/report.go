package parse

import (
	"bufio"
	"fmt"
	"io"
	"sort"
	"strings"
)

func Report(w io.Writer, r *Result, catalog ...map[string]string) error {
	kinds := map[string]int{}
	depths := map[int]int{}
	relations := map[string]int{}
	targets := map[string]int{}
	withdrawn := 0
	unresolved := 0
	dropped := 0
	for i, n := range r.Nodes {
		dropped += n.DroppedMappings
		kind := n.Kind
		switch kind {
		case "family", "control", "enhancement", "function", "category", "subcategory", "safeguard":
		default:
			kind = "other"
		}
		kinds[kind]++
		depth := 0
		for p := n.Parent; p >= 0 && p < i; p = r.Nodes[p].Parent {
			depth++
		}
		depths[depth]++
		if n.Status == "withdrawn" {
			withdrawn++
		}
		for _, m := range n.Mappings {
			if m.Unresolved {
				unresolved++
			}
			rel := m.Relation
			switch rel {
			case "related", "moved-to", "incorporated-into":
			default:
				rel = "other"
			}
			relations[rel]++
			target := "other"
			switch {
			case strings.HasPrefix(strings.ToLower(m.TargetFramework), "nist80053"), strings.HasPrefix(strings.ToLower(m.TargetFramework), "nist sp 800-53"), strings.HasPrefix(strings.ToLower(m.TargetFramework), "sp 800-53 rev"):
				target = "nist80053"
			case strings.HasPrefix(strings.ToLower(m.TargetFramework), "nistcsf"), strings.HasPrefix(strings.ToLower(m.TargetFramework), "nist csf"):
				target = "nistcsf"
			case strings.HasPrefix(strings.ToLower(m.TargetFramework), "ciscontrols"), strings.HasPrefix(strings.ToLower(m.TargetFramework), "cis controls"):
				target = "ciscontrols"
			case strings.HasPrefix(strings.ToLower(m.TargetFramework), "iso/iec 27001"):
				target = "iso27001"
			}

			if len(catalog) > 0 {
				hint := strings.ToLower(m.TargetFramework)
				longest := 0
				for alias, code := range catalog[0] {
					alias = strings.ToLower(alias)
					if len(alias) > longest && (hint == alias || strings.HasPrefix(hint, alias+" ")) && validCode(code) {
						target = code
						longest = len(alias)
					}
				}
			}
			targets[target]++
		}
	}
	out := bufio.NewWriter(w)
	_, _ = fmt.Fprintf(out, "controls: %d\nwithdrawn: %d\ninvalid citations: %d\nduplicates: %d\nunresolved mappings: %d\ndropped mappings: %d\n", len(r.Nodes), withdrawn, r.InvalidCitations, r.Duplicates, unresolved, dropped)
	writeCounts := func(prefix string, m map[string]int) {
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			_, _ = fmt.Fprintf(out, "%s %s: %d\n", prefix, k, m[k])
		}
	}
	writeCounts("kind", kinds)
	ds := make([]int, 0, len(depths))
	for d := range depths {
		ds = append(ds, d)
	}
	sort.Ints(ds)
	for _, d := range ds {
		_, _ = fmt.Fprintf(out, "depth %d: %d\n", d, depths[d])
	}
	writeCounts("relation", relations)
	writeCounts("target", targets)
	return out.Flush()
}

func validCode(s string) bool {
	if s == "" || s[0] < 'a' || s[0] > 'z' {
		return false
	}
	for _, c := range s {
		if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '-' {
			return false
		}
	}
	return true
}
