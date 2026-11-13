package relate

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/stage/chunk"
	"danny.vn/trachuan/apps/corpus/internal/stage/relate/textrule"
	"danny.vn/trachuan/apps/corpus/internal/store"
	"danny.vn/trachuan/internal/retrieval"
)

type relationReport struct {
	Unmapped []string    `json:"unmapped"`
	Text     *textReport `json:"text,omitempty"`
}

type textReport struct {
	Emitted    map[string]int `json:"emitted"`
	Gaps       map[string]int `json:"gaps"`
	GapDetails []textGap      `json:"gap_details"`
}

type textGap struct {
	Name         string `json:"name"`
	Detail       string `json:"detail"`
	SourceNodeID string `json:"source_node_id"`
}

func newTextReport() *textReport {
	return &textReport{Emitted: map[string]int{}, Gaps: map[string]int{}, GapDetails: []textGap{}}
}

func (r *textReport) addGap(name, detail, node string) {
	r.Gaps[name]++
	r.GapDetails = append(r.GapDetails, textGap{Name: name, Detail: detail, SourceNodeID: node})
}

func processText(ctx context.Context, env stage.Env, run stage.Run, item stage.Item, hash string, rows []relation, issuers map[string]string) ([]relation, *textReport, error) {
	input, err := store.New(env.DB).GetRunItemArtifact(ctx, store.GetRunItemArtifactParams{RunID: run.ID, Stage: "extract", ItemKey: item.Key})
	if err != nil {
		return nil, nil, fmt.Errorf("load relate extract artifact: %w", err)
	}
	if input.State != "done" || input.Sha256 != hash {
		return nil, nil, stage.Retryable(fmt.Errorf("relate extract artifact changed for %q", item.Key))
	}
	dir := env.Artifacts.Abs(input.Path)
	if err = verifyFetch(dir, hash); err != nil {
		return nil, nil, fmt.Errorf("verify relate extract: %w", err)
	}
	return textRelations(dir, rows, issuers)
}

func textRelations(dir string, rows []relation, issuers map[string]string) ([]relation, *textReport, error) {
	report := newTextReport()
	var gaps []stage.Gap
	if err := readJSON(filepath.Join(dir, "report.json"), &gaps); err != nil {
		return nil, nil, err
	}
	incomplete := map[string]bool{}
	for _, gap := range gaps {
		switch gap.Kind {
		case "tree_headings_only", "tree_headings_partial", "structure_uncertain", "table_text", "image_content", "ocr_flagged", "ocr_rejected", "ocr_label_flagged", "needs_ocr":
			incomplete[gap.Kind] = true
		}
	}
	if len(incomplete) > 0 {
		names := make([]string, 0, len(incomplete))
		for name := range incomplete {
			names = append(names, name)
		}
		sort.Strings(names)
		report.addGap("relation_text_source_incomplete", strings.Join(names, ", "), "")
		return rows, report, nil
	}
	var provisions []chunk.Node
	if err := readJSON(filepath.Join(dir, "provisions.json"), &provisions); err != nil {
		return nil, nil, err
	}
	targets := map[string][]relation{}
	for _, row := range rows {
		if row.TargetVBPLID != "" {
			number := retrieval.NormalizeNumber(row.TargetNumber)
			targets[number] = append(targets[number], row)
		}
	}
	children := map[string][]chunk.Node{}
	for _, node := range provisions {
		if node.ParentNodeID != nil {
			children[*node.ParentNodeID] = append(children[*node.ParentNodeID], node)
		}
	}
	type identity struct{ node, target, anchor, kind string }
	seen := map[identity]bool{}
	emitted := []relation{}
	for _, article := range provisions {
		if article.Kind != "dieu" || strings.TrimSpace(article.Text) == "" {
			continue
		}
		units, ordered := articleReading(article, children)
		if !ordered {
			report.addGap("relation_text_source_order", article.Text, article.NodeID)
			continue
		}
		matches, parseGaps := textrule.ParseUnits(units, issuers, func(number string) string {
			return targetGap(targets[retrieval.NormalizeNumber(number)])
		})
		for _, gap := range parseGaps {
			report.addGap(gap.Name, gap.Detail, article.NodeID)
		}
		for _, match := range matches {
			for _, target := range match.Targets {
				linked := targets[retrieval.NormalizeNumber(target.Number)]
				if len(linked) != 1 {
					name := "relation_text_unlinked_target"
					if len(linked) > 1 {
						name = "relation_text_ambiguous_target"
					}
					report.addGap(name, target.Number, article.NodeID)
					continue
				}
				key := identity{article.NodeID, linked[0].TargetVBPLID, target.Anchor, match.Kind}
				if seen[key] {
					continue
				}
				seen[key] = true
				node := article.NodeID
				emitted = append(emitted, relation{Kind: match.Kind, RawCode: "text", RawLabel: match.VerbPhrase, Method: "text_rule/1", Evidence: match.Sentence, TargetVBPLID: linked[0].TargetVBPLID, TargetNumber: linked[0].TargetNumber, SourceNodeID: &node, TargetAnchor: target.Anchor, IssuerCode: target.IssuerCode, StatedIssueDate: target.StatedIssueDate})
				report.Emitted[match.Kind]++
			}
		}
	}
	sortRelations(emitted)
	for i := range emitted {
		emitted[i].Ordinal = len(rows) + i
	}
	return append(rows, emitted...), report, nil
}

func (r relationReport) resolveGaps(rows []relation, expected map[string]bool) []stage.Gap {
	gaps := resolveGaps(rows, r.Unmapped, expected)
	if r.Text != nil {
		for i, gap := range r.Text.GapDetails {
			gaps = append(gaps, stage.Gap{Kind: gap.Name, Detail: runGapDetail(i, gap)})
		}
	}
	return gaps
}

func targetGap(linked []relation) string {
	if len(linked) == 0 {
		return "relation_text_unlinked_target"
	}
	if len(linked) > 1 {
		return "relation_text_ambiguous_target"
	}
	return ""
}

func articleReading(article chunk.Node, children map[string][]chunk.Node) ([]textrule.Unit, bool) {
	scoped, _ := textrule.ScopeLayout(article.Text)
	if !scoped {
		return []textrule.Unit{{Text: article.Text, Kind: article.Kind, Close: true}}, true
	}
	units, ok := readTextNode(article, children, map[string]bool{})
	if !ok {
		return nil, false
	}
	if len(units) > 0 {
		units[len(units)-1].Close = true
	}
	return units, true
}

func readTextNode(node chunk.Node, children map[string][]chunk.Node, active map[string]bool) ([]textrule.Unit, bool) {
	if active[node.NodeID] {
		return nil, false
	}
	active[node.NodeID] = true
	defer delete(active, node.NodeID)
	descendants, ok := descendantTexts(node.NodeID, children, map[string]bool{})
	if !ok {
		return nil, false
	}
	text := textrule.CanonicalText(node.Text)
	contained := 0
	for _, desc := range descendants {
		if strings.Contains(text, desc) {
			contained++
		}
	}
	if len(descendants) == 0 || contained == len(descendants) {
		return []textrule.Unit{{Text: node.Text, Kind: node.Kind, Close: node.Kind == "dieu"}}, true
	}
	if contained != 0 {
		return nil, false
	}
	next := "diem"
	if node.Kind == "dieu" {
		_, leadOnly := textrule.ScopeLayout(node.Text)
		if !leadOnly {
			return nil, false
		}
		next = "khoan"
	} else if node.Kind != "khoan" {
		return nil, false
	}
	direct := append([]chunk.Node(nil), children[node.NodeID]...)
	sort.Slice(direct, func(i, j int) bool { return direct[i].Ordinal < direct[j].Ordinal })
	units := []textrule.Unit{{Text: node.Text, Kind: node.Kind, Close: node.Kind == "dieu"}}
	for i, child := range direct {
		if child.Kind != next || (i > 0 && direct[i-1].Ordinal == child.Ordinal) {
			return nil, false
		}
		part, valid := readTextNode(child, children, active)
		if !valid {
			return nil, false
		}
		if node.Kind == "dieu" && len(part) > 0 {
			part[0].FirstItem = true
			part[len(part)-1].Close = true
		}
		units = append(units, part...)
	}
	return units, true
}

func descendantTexts(id string, children map[string][]chunk.Node, active map[string]bool) ([]string, bool) {
	if active[id] {
		return nil, false
	}
	active[id] = true
	defer delete(active, id)
	var out []string
	for _, child := range children[id] {
		if text := textrule.CanonicalText(child.Text); text != "" {
			out = append(out, text)
		}
		more, ok := descendantTexts(child.NodeID, children, active)
		if !ok {
			return nil, false
		}
		out = append(out, more...)
	}
	return out, true
}
