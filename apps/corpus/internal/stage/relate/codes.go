package relate

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/dannyota/traquydinh/apps/corpus/internal/source"
	"github.com/dannyota/traquydinh/apps/corpus/internal/stage"
)

type relation struct {
	Ordinal      int     `json:"ordinal"`
	Kind         string  `json:"kind"`
	RawCode      string  `json:"raw_code"`
	RawLabel     string  `json:"raw_label"`
	Method       string  `json:"method"`
	Evidence     string  `json:"evidence"`
	TargetVBPLID string  `json:"target_vbpl_id"`
	TargetNumber string  `json:"target_number"`
	SourceNodeID *string `json:"source_node_id"`
	TargetNodeID *string `json:"target_node_id"`
}

type identifier string

func (i *identifier) UnmarshalJSON(b []byte) error {
	if string(b) == "null" {
		*i = ""
		return nil
	}
	var s string
	if len(b) > 0 && b[0] == '"' {
		if err := json.Unmarshal(b, &s); err != nil {
			return fmt.Errorf("decode identifier: %w", err)
		}
	} else {
		var n json.Number
		if err := json.Unmarshal(b, &n); err != nil {
			return fmt.Errorf("decode identifier: %w", err)
		}
		s = n.String()
	}
	*i = identifier(strings.TrimSpace(s))
	return nil
}

func relations(detail, diagram []byte, codes map[string]source.RelationCode) ([]relation, []string, error) {
	var d struct {
		Data struct {
			References []struct {
				ReferenceType identifier `json:"referenceType"`
				Target        struct {
					ID     identifier `json:"id"`
					DocNum string     `json:"docNum"`
					Title  string     `json:"title"`
				} `json:"targetDocument"`
				SourceNodeID *string `json:"sourceNodeId"`
				TargetNodeID *string `json:"targetNodeId"`
			} `json:"references"`
		} `json:"data"`
	}
	if err := json.Unmarshal(detail, &d); err != nil {
		return nil, nil, fmt.Errorf("decode detail: %w", err)
	}
	var diagramData struct {
		Data struct {
			ByType map[string][]struct {
				ID   identifier `json:"id"`
				Name string     `json:"name"`
			} `json:"documentNamesByType"`
		} `json:"data"`
	}
	if err := json.Unmarshal(diagram, &diagramData); err != nil {
		return nil, nil, fmt.Errorf("decode diagram: %w", err)
	}
	rows := make([]relation, 0)
	indexes := make(map[string]int)
	add := func(row relation) {
		target := "id:" + row.TargetVBPLID
		if row.TargetVBPLID == "" {
			target = "number:" + row.TargetNumber
		}
		key := row.RawCode + "\x00" + target
		if index, ok := indexes[key]; ok {
			existing := &rows[index]
			if existing.Method != row.Method && !strings.Contains(existing.Method, row.Method) {
				existing.Method += "," + row.Method
			}
			if existing.TargetNumber == "" {
				existing.TargetNumber = row.TargetNumber
			}
			return
		}
		indexes[key] = len(rows)
		rows = append(rows, row)
	}
	for _, ref := range d.Data.References {
		add(relation{RawCode: string(ref.ReferenceType), Method: "vbpl_references", Evidence: ref.Target.Title, TargetVBPLID: string(ref.Target.ID), TargetNumber: strings.TrimSpace(ref.Target.DocNum), SourceNodeID: ref.SourceNodeID, TargetNodeID: ref.TargetNodeID})
	}
	for code, docs := range diagramData.Data.ByType {
		for _, doc := range docs {
			add(relation{RawCode: code, Method: "vbpl_diagram", Evidence: doc.Name, TargetVBPLID: string(doc.ID), TargetNumber: documentNumber(doc.Name)})
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		if a.RawCode != b.RawCode {
			return a.RawCode < b.RawCode
		}
		if a.TargetVBPLID != b.TargetVBPLID {
			return a.TargetVBPLID < b.TargetVBPLID
		}
		return a.TargetNumber < b.TargetNumber
	})
	missing := make(map[string]bool)
	for i := range rows {
		rows[i].Ordinal = i
		if code, ok := codes[rows[i].RawCode]; ok {
			rows[i].Kind = code.Kind
			rows[i].RawLabel = code.Label
		} else {
			rows[i].Kind = "unknown"
			missing[rows[i].RawCode] = true
		}
	}
	unmapped := make([]string, 0, len(missing))
	for code := range missing {
		unmapped = append(unmapped, code)
	}
	sort.Strings(unmapped)
	return rows, unmapped, nil
}

func documentNumber(name string) string {
	pattern := regexp.MustCompile(`(?i)\b[0-9]{1,4}\s*/\s*(?:[0-9]{4}\s*/\s*)?[\pL][\pL0-9]*(?:\s*-\s*[\pL0-9]+)*`)
	separators := regexp.MustCompile(`\s*([/-])\s*`)
	return strings.ToUpper(separators.ReplaceAllString(pattern.FindString(name), "$1"))
}

func resolveGaps(rows []relation, unmapped []string, expected map[string]bool) []stage.Gap {
	var gaps []stage.Gap
	for _, code := range unmapped {
		gaps = append(gaps, stage.Gap{Kind: "unknown_relation_code", Detail: code})
	}
	for _, row := range rows {
		if row.TargetVBPLID == "" || !expected["vn:vbpl:"+row.TargetVBPLID] {
			detail := row.TargetVBPLID
			if detail == "" {
				detail = row.TargetNumber
			}
			gaps = append(gaps, stage.Gap{Kind: "unknown_target", Detail: detail})
		}
	}
	return gaps
}
