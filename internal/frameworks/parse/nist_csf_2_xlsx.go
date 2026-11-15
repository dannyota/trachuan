package parse

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	csfFunctionPattern     = regexp.MustCompile(`^[A-Z]+\s+\(([A-Z]{2})\)(?::\s*(.*))?$`)
	csfCategoryPattern     = regexp.MustCompile(`^(.+?)\s+\(([A-Z]{2}\.[A-Z]{2,})\):\s*(.+)$`)
	csfSubcategoryPattern  = regexp.MustCompile(`^([A-Z]{2}\.[A-Z]{2,}-[0-9]+):\s*(.+)$`)
	csfWithdrawalPattern   = regexp.MustCompile(`^[A-Z]{2}(?:\.[A-Z]{2,}(?:-[0-9]+)?)?$`)
	csfVersionPattern      = regexp.MustCompile(`^[0-9]{4}$`)
	csfISOReferencePattern = regexp.MustCompile(`^[0-9]+(?:\.[0-9]+)*$`)
)

func parseCSF(data []byte, code, version string) ([]Node, error) {
	rows, err := rows(data, "CSF 2.0", false)
	if err != nil {
		return nil, err
	}
	var nodes []Node
	indexes := map[string]int{}
	currentFunction := -1
	for _, r := range rows[min(2, len(rows)):] {
		a, b, c := cell(r, 0), cell(r, 1), cell(r, 2)
		var citation, kind, title, body string
		parent := -1
		switch {
		case a != "":
			m := csfFunctionPattern.FindStringSubmatch(a)
			if m == nil {
				return nodes, fmt.Errorf("invalid function row")
			}
			citation = m[1]
			kind = "function"
			title = m[2]
			if idx, ok := indexes[citation]; ok {
				currentFunction = idx
				if nodes[idx].PublisherTitle == "" && title != "" {
					nodes[idx].PublisherTitle = title
					nodes[idx].Mappings, nodes[idx].DroppedMappings = references(cell(r, 4))
				}
				continue
			}
		case b != "":
			m := csfCategoryPattern.FindStringSubmatch(b)
			if m == nil {
				return nodes, fmt.Errorf("invalid category row")
			}
			citation = m[2]
			kind = "category"
			title = m[1]
			body = m[3]
			key := strings.Split(citation, ".")[0]
			var ok bool
			parent, ok = indexes[key]
			if !ok {
				parent = currentFunction
			}
			if parent < 0 {
				return nodes, fmt.Errorf("missing category parent")
			}
		case c != "":
			m := csfSubcategoryPattern.FindStringSubmatch(c)
			if m == nil {
				return nodes, fmt.Errorf("invalid subcategory row")
			}
			citation = m[1]
			kind = "subcategory"
			title = m[2]
			body = m[2]
			if extra := cell(r, 3); extra != "" {
				body += "\n\n" + extra
			}
			key := strings.Split(citation, "-")[0]
			var ok bool
			parent, ok = indexes[key]
			if !ok {
				parent = -1
				if strings.HasPrefix(m[2], "[Withdrawn:") {
					parent = currentFunction
				}
			}
			if parent < 0 {
				return nodes, fmt.Errorf("missing subcategory parent")
			}
		default:
			continue
		}
		n := Node{Citation: citation, Kind: kind, Parent: parent, Ordinal: len(nodes), PublisherTitle: title, Body: body, Status: "active"}
		if strings.HasPrefix(body, "[Withdrawn:") {
			n.Status = "withdrawn"
			noteText, _, _ := strings.Cut(body, "\n")
			note := strings.TrimSuffix(strings.TrimPrefix(noteText, "[Withdrawn:"), "]")
			note = strings.TrimSpace(note)
			for _, action := range []struct{ prefix, relation string }{{"Incorporated into ", "incorporated-into"}, {"Moved to ", "moved-to"}, {"Moved into ", "moved-to"}} {
				if strings.HasPrefix(note, action.prefix) {
					for _, target := range strings.Split(strings.TrimPrefix(note, action.prefix), ",") {
						target = strings.TrimSpace(target)
						if csfWithdrawalPattern.MatchString(target) {
							n.Mappings = append(n.Mappings, Mapping{TargetFramework: code, TargetVersion: version, TargetCitation: target, Relation: action.relation})
						} else {
							n.DroppedMappings++
						}
					}
					break
				}
			}
		}
		refs, dropped := references(cell(r, 4))
		n.Mappings = append(n.Mappings, refs...)
		n.DroppedMappings += dropped
		indexes[citation] = len(nodes)
		if kind == "function" {
			currentFunction = len(nodes)
		}
		nodes = append(nodes, n)
	}
	return nodes, nil
}

func references(value string) ([]Mapping, int) {
	var mappings []Mapping
	dropped := 0
	for _, line := range strings.Split(value, "\n") {
		target, citation, ok := strings.Cut(line, ":")
		if !ok {
			if strings.TrimSpace(line) == "ISO/IEC 27001" {
				dropped++
			}
			continue
		}
		target, citation = strings.TrimSpace(target), strings.TrimSpace(citation)
		if target == "" {
			continue
		}
		mapping := reference(target, citation)
		if mapping.TargetCitation == "" {
			if target == "ISO/IEC 27001" {
				dropped++
			}
			continue
		}
		mappings = append(mappings, mapping)
	}
	return mappings, dropped
}

func reference(target, citation string) Mapping {
	m := Mapping{TargetFramework: target, TargetCitation: citation, Relation: "related"}
	if target != "ISO/IEC 27001" {
		return m
	}
	m.TargetCitation = ""
	version, rest, ok := strings.Cut(citation, ":")
	if !ok || !csfVersionPattern.MatchString(strings.TrimSpace(version)) {
		return m
	}
	m.TargetVersion = strings.TrimSpace(version)
	rest = strings.TrimSpace(rest)
	annex := false
	switch {
	case strings.HasPrefix(rest, "Annex A Controls:"):
		citation = strings.TrimSpace(strings.TrimPrefix(rest, "Annex A Controls:"))
		annex = true
	case strings.HasPrefix(rest, "Mandatory Clause:"):
		citation = strings.TrimSpace(strings.TrimPrefix(rest, "Mandatory Clause:"))
	case strings.HasPrefix(rest, "Control "):
		citation = strings.TrimSpace(strings.TrimPrefix(rest, "Control "))
		annex = true
	default:
		return m
	}
	citation = strings.TrimSpace(strings.TrimSuffix(citation, ","))
	if !csfISOReferencePattern.MatchString(citation) {
		return m
	}
	if annex {
		citation = "A." + citation
	}
	m.TargetCitation = citation
	return m
}
