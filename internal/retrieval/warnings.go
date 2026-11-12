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
