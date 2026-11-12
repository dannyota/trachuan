package retrieval

import "strings"

// Warning identifies an act that targets a provision and its source basis.
type Warning struct {
	Pointer
	Source   string `json:"source,omitempty"`
	Basis    string `json:"basis"`
	Target   string `json:"target"`
	Citation string `json:"citation"`
	Exact    bool   `json:"exact"`
	Named    string `json:"named,omitempty"`
}

func warning(r Relation) (Warning, bool) {
	switch r.Kind {
	case "repeals", "replaces", "suspends", "amends_supplements":
	default:
		return Warning{}, false
	}
	if !r.TargetResolved || r.TargetAnchor == nil || r.TargetNodeID == nil {
		return Warning{}, false
	}
	methods := strings.Split(r.Method, ",")
	text, reference := false, false
	for _, method := range methods {
		switch method {
		case "text_rule/1":
			text = true
		case "vbpl_references":
			reference = true
		}
	}
	if !text && !reference {
		return Warning{}, false
	}
	w := Warning{Pointer: Pointer{Dir: "in", Kind: r.Kind, Key: r.SourceKey, Number: r.SourceNumber, InRelease: r.SourceResolved}, Target: r.TargetDocKey + "#" + *r.TargetAnchor, Citation: r.TargetCitation}
	if r.SourceResolved {
		w.Key = r.SourceDocKey
		if r.SourceAnchor != nil {
			w.Source = r.SourceDocKey + "#" + *r.SourceAnchor
		}
	}
	if reference {
		w.Basis = "vbpl_reference"
		w.Exact = true
	} else {
		w.Basis = "text"
		w.Exact = *r.TargetNodeID == *r.TargetAnchor
		if !w.Exact {
			w.Named = *r.TargetNodeID
		}
	}
	return w, true
}

func shapeWarnings(source []SourceRelation) ([]Warning, int) {
	var warnings []Warning
	seen := map[struct {
		source, hash string
		ordinal      int
	}]bool{}
	omitted := 0
	for _, e := range orderedRelations(source) {
		if !e.TargetInHit {
			continue
		}
		w, ok := warning(e.Value)
		if !ok {
			continue
		}
		id := struct {
			source, hash string
			ordinal      int
		}{e.Value.SourceKey, e.ArtifactSHA256, e.Ordinal}
		if seen[id] {
			continue
		}
		seen[id] = true
		if len(warnings) == 10 {
			omitted++
			continue
		}
		warnings = append(warnings, w)
	}
	return warnings, omitted
}
