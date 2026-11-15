package extract

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"danny.vn/trachuan/apps/corpus/internal/textparse"
	"danny.vn/trachuan/apps/corpus/internal/vbpl"
)

func partialTreeSelection(rows []provision, nodes []vbpl.Node, transcription, kept selection) (selection, []vbpl.Node, error) {
	treeChars, transcriptionChars, accepted, usable := partialTreeLengths(rows, transcription)
	if !usable || treeChars*2 >= transcriptionChars {
		return kept, nodes, nil
	}
	if !accepted {
		detail := fmt.Sprintf("tree_chars=%d transcription_chars=%d", treeChars, transcriptionChars)
		if kept.gap == "" {
			kept.gap, kept.reason = "tree_text_partial", detail
		} else {
			kept.extraGap, kept.extraReason = "tree_text_partial", detail
		}
		return kept, nodes, nil
	}
	transcription.tree = &treeComparison{SetAside: true, Rule: "partial_tree", TreeChars: &treeChars, TranscriptionChars: &transcriptionChars}
	return transcription, nil, nil
}

func partialTreeLengths(rows []provision, transcription selection) (int, int, bool, bool) {
	if !validTreeIdentity(rows) {
		return 0, 0, false, false
	}
	rendered := renderTreeText(rows)
	var texts []string
	for _, row := range rows {
		if row.ParentNodeID == nil {
			texts = append(texts, rendered[row.NodeID])
		}
	}
	treeChars := utf8.RuneCountInString(collapse(strings.Join(texts, " ")))
	if transcription.source != "html_transcription" || transcription.tooLarge || transcription.parseLimit {
		return treeChars, 0, false, false
	}
	paragraphs, err := textparse.ParagraphsFromHTML(transcription.data)
	if err != nil {
		return treeChars, 0, false, false
	}
	texts = nil
	for _, p := range paragraphs {
		texts = append(texts, p.Text)
	}
	transcriptionChars := utf8.RuneCountInString(collapse(strings.Join(texts, " ")))
	result := textparse.Parse(paragraphs)
	accepted := false
	for _, scope := range result.Scopes {
		if scope.Kind == "main" && scope.Accepted {
			accepted = true
		}
	}
	return treeChars, transcriptionChars, accepted, true
}
