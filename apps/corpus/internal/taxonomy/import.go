package taxonomy

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Import assigns stable codes and records removed nodes as retired.
func Import(pkg Package, previous Mapping) (Mapping, error) {
	m := Mapping{PackageSHA256: pkg.SHA256, Nodes: []MappedNode{}, Flattened: []Flattened{}}
	active := map[string]MappedNode{}
	hadPrevious := map[string]bool{}
	used := map[string]bool{}
	max := map[string]int{}
	seen := map[string]bool{}
	for _, n := range previous.Nodes {
		if n.GUID == "" || n.Code == "" || used[n.Code] {
			return m, fmt.Errorf("invalid previous mapping")
		}
		if n.Status == "active" {
			if active[n.GUID].GUID != "" {
				return m, fmt.Errorf("duplicate active GUID %s", n.GUID)
			}
			active[n.GUID] = n
		}
		hadPrevious[n.GUID] = true
		used[n.Code] = true
		reserve(max, n.Code)
	}
	add := func(n MappedNode) error {
		if n.GUID == "" || seen[n.GUID] {
			return fmt.Errorf("duplicate or empty GUID %q", n.GUID)
		}
		seen[n.GUID] = true
		if prev, ok := active[n.GUID]; ok {
			n.Code = prev.Code
		} else if used[n.Code] {
			return fmt.Errorf("code %s already used", n.Code)
		}
		used[n.Code] = true
		reserve(max, n.Code)
		m.Nodes = append(m.Nodes, n)
		return nil
	}
	subjects := append([]ChuDe(nil), pkg.ChuDe...)
	sort.SliceStable(subjects, func(i, j int) bool { return subjects[i].STT < subjects[j].STT })
	codes := map[string]string{}
	statuses := map[string]string{}
	ordinal := map[string]int{}
	for _, s := range subjects {
		if s.STT < 1 {
			return m, fmt.Errorf("invalid subject STT")
		}
		code := strconv.Itoa(s.STT)
		if _, ok := active[s.GUID]; !ok && used[code] {
			code = strconv.Itoa(max[""] + 1)
		}
		n := MappedNode{GUID: s.GUID, Code: code, Level: 1, Kind: "chude", Name: cleanName(s.Name), Status: "active"}
		ordinal[""]++
		n.Ordinal = ordinal[""]
		if err := add(n); err != nil {
			return m, err
		}
		codes[s.GUID] = m.Nodes[len(m.Nodes)-1].Code
	}
	sections := append([]DeMuc(nil), pkg.DeMuc...)
	sort.SliceStable(sections, func(i, j int) bool {
		if sections[i].ChuDeGUID != sections[j].ChuDeGUID {
			return sections[i].ChuDeGUID < sections[j].ChuDeGUID
		}
		return sections[i].STT < sections[j].STT
	})
	// Assign collision codes in package order before sorting for display ordinals.
	for _, d := range pkg.DeMuc {
		parent, ok := codes[d.ChuDeGUID]
		if !ok || d.STT < 1 {
			return m, fmt.Errorf("invalid section %s", d.GUID)
		}
		status := "active"
		if d.Removed {
			status = "retired"
			if _, ok := active[d.GUID]; !ok && hadPrevious[d.GUID] {
				if seen[d.GUID] {
					return m, fmt.Errorf("duplicate or empty GUID %q", d.GUID)
				}
				seen[d.GUID] = true
				statuses[d.GUID] = status
				continue
			}
		}
		code := parent + "." + strconv.Itoa(d.STT)
		if _, ok := active[d.GUID]; !ok && used[code] {
			code = nextCode(parent, "", max)
		}
		n := MappedNode{GUID: d.GUID, Code: code, ParentCode: parent, Level: 2, Kind: "demuc", Name: cleanName(strings.TrimSuffix(d.Name, " (loại bỏ)")), Status: status}
		if err := add(n); err != nil {
			return m, err
		}
		codes[d.GUID] = m.Nodes[len(m.Nodes)-1].Code
		statuses[d.GUID] = status
	}
	indices := map[string]int{}
	for i, n := range m.Nodes {
		indices[n.GUID] = i
	}
	for _, d := range sections {
		i, ok := indices[d.GUID]
		if ok && m.Nodes[i].Status == "active" {
			ordinal[m.Nodes[i].ParentCode]++
			m.Nodes[i].Ordinal = ordinal[m.Nodes[i].ParentCode]
		}
	}
	tree := map[string]TreeNode{}
	children := map[string][]TreeNode{}
	for _, n := range pkg.Tree {
		if n.GUID == "" || tree[n.GUID].GUID != "" {
			return m, fmt.Errorf("duplicate tree GUID %q", n.GUID)
		}
		tree[n.GUID] = n
		children[n.ParentGUID] = append(children[n.ParentGUID], n)
	}
	for id := range children {
		sort.SliceStable(children[id], func(i, j int) bool {
			a, b := children[id][i], children[id][j]
			if a.Order != b.Order {
				return a.Order < b.Order
			}
			return a.MAPC < b.MAPC
		})
	}
	var walk func(TreeNode, string, int, string) error
	walk = func(t TreeNode, parent string, level int, ancestor string) error {
		if seen[t.GUID] {
			return fmt.Errorf("duplicate tree GUID %s", t.GUID)
		}
		if t.Kind == KindDieu {
			seen[t.GUID] = true
			for _, c := range children[t.GUID] {
				if c.DeMucGUID == t.DeMucGUID {
					if err := walk(c, parent, level, ancestor); err != nil {
						return err
					}
				}
			}
			return nil
		}
		reason := ""
		switch t.Kind {
		case KindTieuMuc:
			if ancestor != "muc" && ancestor != "chuong" {
				reason = "no_structural_parent"
			} else {
				reason = "tieu_muc"
			}
		case KindMuc:
			if ancestor != "muc" && ancestor != "chuong" {
				reason = "no_structural_parent"
			} else if ancestor == "muc" {
				reason = "nested_muc"
			} else if level == 4 {
				reason = "muc_under_level4_chuong"
			}
		case KindPhan, KindChuong:
		default:
			return fmt.Errorf("unknown tree kind %q", t.Kind)
		}
		if reason == "" && level >= 4 {
			reason = "level5"
		}
		nextParent, nextLevel, nextAncestor := parent, level, ancestor
		if reason != "" {
			seen[t.GUID] = true
			m.Flattened = append(m.Flattened, Flattened{GUID: t.GUID, Into: parent, Reason: reason})
			if t.Kind == KindMuc {
				nextAncestor = "muc"
			}
		} else {
			kind := string(t.Kind)
			prefix := map[NodeKind]string{KindPhan: "p", KindChuong: "c", KindMuc: "m"}[t.Kind]
			code := nextCode(parent, prefix, max)
			n := MappedNode{GUID: t.GUID, Code: code, ParentCode: parent, Level: level + 1, Kind: kind, Name: cleanName(t.Name), Status: "active"}
			ordinal[parent]++
			n.Ordinal = ordinal[parent]
			if err := add(n); err != nil {
				return err
			}
			nextParent = m.Nodes[len(m.Nodes)-1].Code
			nextLevel = level + 1
			nextAncestor = kind
		}
		for _, c := range children[t.GUID] {
			if c.DeMucGUID == t.DeMucGUID {
				if err := walk(c, nextParent, nextLevel, nextAncestor); err != nil {
					return err
				}
			}
		}
		return nil
	}
	for _, t := range pkg.Tree {
		if statuses[t.DeMucGUID] == "retired" {
			if seen[t.GUID] {
				return m, fmt.Errorf("duplicate tree GUID %s", t.GUID)
			}
			seen[t.GUID] = true
		}
	}
	for _, d := range pkg.DeMuc {
		if d.Removed {
			continue
		}
		for _, t := range children[""] {
			if t.DeMucGUID == d.GUID {
				if err := walk(t, codes[d.GUID], 2, "demuc"); err != nil {
					return m, err
				}
			}
		}
	}
	for _, t := range pkg.Tree {
		if !seen[t.GUID] {
			return m, fmt.Errorf("unreachable tree node %s", t.GUID)
		}
	}
	presentCodes := map[string]bool{}
	for _, n := range m.Nodes {
		presentCodes[n.Code] = true
	}
	for _, n := range previous.Nodes {
		if !presentCodes[n.Code] {
			if n.Status == "active" {
				n.Status = "retired"
				n.Ordinal = 0
			}
			m.Nodes = append(m.Nodes, n)
		}
	}
	return m, nil
}
func cleanName(s string) string { return strings.Join(strings.Fields(s), " ") }
func reserve(max map[string]int, code string) {
	i := strings.LastIndex(code, ".")
	if i < 0 {
		n, err := strconv.Atoi(code)
		if err == nil && n > max[""] {
			max[""] = n
		}
		return
	}
	segment := code[i+1:]
	prefix := ""
	if len(segment) > 0 && (segment[0] < '0' || segment[0] > '9') {
		prefix = segment[:1]
		segment = segment[1:]
	}
	n, err := strconv.Atoi(segment)
	if err == nil && n > max[code[:i]+"."+prefix] {
		max[code[:i]+"."+prefix] = n
	}
}
func nextCode(parent, prefix string, max map[string]int) string {
	key := parent + "." + prefix
	return key + strconv.Itoa(max[key]+1)
}
