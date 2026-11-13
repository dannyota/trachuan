package extract

import (
	"fmt"
	"strings"

	"danny.vn/trachuan/apps/corpus/internal/textparse"
)

type ruleCount struct {
	Total   int `json:"total"`
	Matched int `json:"matched"`
}

type treeComparison struct {
	Articles            int          `json:"articles"`
	HeadingOnlyArticles int          `json:"heading_only_articles"`
	SetAside            bool         `json:"set_aside"`
	FailedRule          int          `json:"failed_rule,omitempty"`
	Rules               [5]ruleCount `json:"rules"`
}

func collapse(s string) string { return strings.Join(strings.Fields(s), " ") }

func headingTextOnly(row provision) bool {
	text, label, heading := collapse(row.Text), collapse(row.Label), collapse(row.Heading)
	if text == "" && row.Kind != "dieu" {
		return true
	}
	if heading != "" && (text == label || text == label+".") {
		return false
	}
	return text == label || text == label+"." || text == label+" "+heading || text == label+". "+heading
}

func headingArticles(rows []provision) (int, map[string]bool) {
	children := make(map[string]bool)
	for _, row := range rows {
		if row.ParentNodeID != nil {
			children[*row.ParentNodeID] = true
		}
	}
	articles := 0
	headings := make(map[string]bool)
	for _, row := range rows {
		if row.Kind != "dieu" {
			continue
		}
		articles++
		if !children[row.NodeID] && headingTextOnly(row) {
			headings[row.NodeID] = true
		}
	}
	return articles, headings
}

func compareTree(rows []provision, selected selection) *treeComparison {
	if !validTreeIdentity(rows) {
		comparison := &treeComparison{FailedRule: 3}
		for _, row := range rows {
			if row.Kind == "dieu" {
				comparison.Articles++
			}
		}
		return comparison
	}
	articles, headings := headingArticles(rows)
	c := &treeComparison{Articles: articles, HeadingOnlyArticles: len(headings)}
	c.Rules[0].Total = 1
	c.Rules[1].Total = articles
	c.Rules[2].Total = articles - len(headings)
	for _, row := range rows {
		switch row.Kind {
		case "khoan", "diem":
			c.Rules[3].Total++
		case "dieu":
		default:
			c.Rules[4].Total++
		}
	}
	var parsed []textparse.Provision
	if selected.source == "html_transcription" && !selected.tooLarge && !selected.parseLimit {
		_, _, record := parseTranscription(selected.data, selected.name, selected.hash)
		for _, scope := range record.Scopes {
			if scope.Kind == "main" && scope.Accepted {
				c.Rules[0].Matched = 1
			}
		}
		parsed = record.Provisions
		accepted := make([]textparse.Provision, 0, len(parsed))
		for _, p := range parsed {
			if p.ScopeIndex >= 0 && p.ScopeIndex < len(record.Scopes) && record.Scopes[p.ScopeIndex].Accepted {
				accepted = append(accepted, p)
			}
		}
		parsed = accepted
		// Rule 5 counts all emitted text, including neutral provisions, but no discarded paragraphs.
		var emitted []string
		for _, p := range record.Provisions {
			emitted = append(emitted, collapse(p.Text))
		}
		text := strings.Join(emitted, " ")
		for _, row := range rows {
			switch row.Kind {
			case "dieu", "khoan", "diem":
			default:
				if strings.Contains(text, collapse(row.Text)) {
					c.Rules[4].Matched++
				}
			}
		}
	}
	var treeArticles []provision
	for _, row := range rows {
		if row.Kind == "dieu" {
			treeArticles = append(treeArticles, row)
		}
	}
	var parsedArticles []textparse.Provision
	anchors := make(map[string]bool)
	for _, p := range parsed {
		switch p.Kind {
		case "dieu":
			parsedArticles = append(parsedArticles, p)
		case "khoan", "diem":
			anchors[p.Anchor] = true
		}
	}
	if len(parsedArticles) > articles {
		c.Rules[1].Total = len(parsedArticles)
	}
	rendered := renderTreeText(rows)
	byAnchor := make(map[string]string)
	for _, p := range parsedArticles {
		byAnchor[p.Anchor] = collapse(p.Text)
	}
	for i, row := range treeArticles {
		if i < len(parsedArticles) && !strings.Contains(row.Path, "~") && row.Path == parsedArticles[i].Anchor {
			c.Rules[1].Matched++
		}
		if !headings[row.NodeID] {
			text, found := byAnchor[row.Path]
			if found && strings.Contains(text, collapse(rendered[row.NodeID])) {
				c.Rules[2].Matched++
			}
		}
	}
	for _, row := range rows {
		if (row.Kind == "khoan" || row.Kind == "diem") && anchors[row.Path] {
			c.Rules[3].Matched++
		}
	}
	for i, rule := range c.Rules {
		if rule.Matched != rule.Total {
			c.FailedRule = i + 1
			break
		}
	}
	c.SetAside = c.FailedRule == 0
	return c
}

func (c *treeComparison) detail() string {
	return fmt.Sprintf("%d of %d articles heading-only; rule %d failed", c.HeadingOnlyArticles, c.Articles, c.FailedRule)
}

func validTreeIdentity(rows []provision) bool {
	ids := make(map[string]bool, len(rows))
	for _, row := range rows {
		if row.NodeID == "" || ids[row.NodeID] {
			return false
		}
		ids[row.NodeID] = true
	}
	for _, row := range rows {
		if row.ParentNodeID != nil && !ids[*row.ParentNodeID] {
			return false
		}
	}
	return true
}
