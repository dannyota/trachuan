package extract

import (
	"errors"
	"strings"

	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/textparse"
)

type textRecord struct {
	TextSource string                `json:"text_source"`
	FileName   string                `json:"file_name"`
	SHA256     string                `json:"sha256"`
	Paragraphs []textparse.Paragraph `json:"paragraphs"`
	Scopes     []textparse.Scope     `json:"scopes"`
	Counts     textparse.Counts      `json:"counts"`
	Provisions []textparse.Provision `json:"provisions"`
}

func parseTranscription(data []byte, name, hash string) ([]provision, []stage.Gap, *textRecord) {
	rows := []provision{}
	gaps := []stage.Gap{}
	paragraphs, err := textparse.ParagraphsFromHTML(data)
	record := &textRecord{TextSource: "html_transcription", FileName: name, SHA256: hash, Paragraphs: paragraphs}
	if err != nil {
		var limit *textparse.LimitError
		if errors.As(err, &limit) {
			gaps = append(gaps, stage.Gap{Kind: "text_limit", Detail: limit.Error()})
		} else {
			gaps = append(gaps, stage.Gap{Kind: "no_text", Detail: err.Error()})
		}
	} else {
		result := textparse.Parse(paragraphs)
		record.Scopes = result.Scopes
		record.Counts = result.Counts
		record.Provisions = result.Provisions
		var part, chapter, section *string
		scope := -2
		for _, p := range result.Provisions {
			if p.ScopeIndex != scope {
				part, chapter, section = nil, nil, nil
				scope = p.ScopeIndex
			}
			label, heading := p.Label, p.Heading
			if p.Kind == "phan" || p.Kind == "chuong" || p.Kind == "muc" {
				lines := strings.SplitN(p.Text, "\n", 2)
				label, heading, _, _ = titleParts(p.Kind, lines[0], len(rows)+1)
				if heading == "" && len(lines) > 1 {
					heading = lines[1]
				}
			}
			var parent *string
			switch p.Kind {
			case "phan":
				part, chapter, section = nil, nil, nil
			case "chuong":
				parent = part
				chapter, section = nil, nil
			case "muc":
				parent = chapter
				if parent == nil {
					parent = part
				}
				section = nil
			default:
				parent = section
				if parent == nil {
					parent = chapter
				}
				if parent == nil {
					parent = part
				}
			}
			rows = append(rows, provision{NodeID: p.NodeID, ParentNodeID: parent, Kind: p.Kind, Label: label, Heading: heading, Ordinal: len(rows) + 1, Path: p.Anchor, Text: p.Text})
			id := p.NodeID
			switch p.Kind {
			case "phan":
				part = &id
			case "chuong":
				chapter = &id
			case "muc":
				section = &id
			}
		}
		for _, gap := range result.Gaps {
			detail := ""
			if gap == "structure_uncertain" {
				var failures []string
				for _, scope := range result.Scopes {
					if scope.FailedCheck != "" {
						failures = append(failures, scope.FailedCheck)
					}
				}
				detail = strings.Join(failures, "; ")
			}
			gaps = append(gaps, stage.Gap{Kind: gap, Detail: detail})
		}
	}
	if len(rows) == 0 {
		gaps = append(gaps, stage.Gap{Kind: "no_tree"})
	}
	return rows, gaps, record
}
