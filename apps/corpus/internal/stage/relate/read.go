package relate

import (
	"strings"

	"danny.vn/trachuan/apps/corpus/internal/stage/chunk"
	"danny.vn/trachuan/apps/corpus/internal/stage/heading"
	"danny.vn/trachuan/apps/corpus/internal/stage/relate/textrule"
)

func IncompleteGap(kind string) bool {
	switch kind {
	case "tree_headings_only", "tree_headings_partial", "tree_text_partial", "structure_uncertain", "table_text", "image_content", "ocr_flagged", "ocr_rejected", "ocr_label_flagged", "needs_ocr":
		return true
	}
	return false
}

func BlockingGap(kind string) bool {
	return IncompleteGap(kind) && kind != "tree_headings_only" && kind != "tree_headings_partial"
}

type ArticleState int

const (
	ArticleRead ArticleState = iota
	ArticleEmpty
	ArticleHeadingOnly
	ArticleUnordered
)

func WalkArticles(nodes []chunk.Node, headingsPartial bool, visit func(article chunk.Node, state ArticleState, units []textrule.Unit)) {
	children := map[string][]chunk.Node{}
	for _, node := range nodes {
		if node.ParentNodeID != nil {
			children[*node.ParentNodeID] = append(children[*node.ParentNodeID], node)
		}
	}
	for _, article := range nodes {
		switch {
		case article.Kind != "dieu":
			continue
		case strings.TrimSpace(article.Text) == "":
			visit(article, ArticleEmpty, nil)
			continue
		case headingsPartial && heading.ArticleOnly(article.Kind, article.Label, article.Heading, article.Text, len(children[article.NodeID]) > 0):
			visit(article, ArticleHeadingOnly, nil)
			continue
		}
		units, ordered := articleReading(article, children)
		if !ordered {
			visit(article, ArticleUnordered, nil)
			continue
		}
		visit(article, ArticleRead, units)
	}
}

type ArticleReading struct {
	NodeID  string
	Matches []textrule.Match
}

type ArticleReport struct {
	Readings []ArticleReading
	Unread   []string
}

func ReadArticles(nodes []chunk.Node, issuers map[string]string, headingsPartial bool) ArticleReport {
	var report ArticleReport
	WalkArticles(nodes, headingsPartial, func(article chunk.Node, state ArticleState, units []textrule.Unit) {
		switch state {
		case ArticleRead:
			if matches, _ := textrule.ParseUnits(units, issuers, nil); len(matches) > 0 {
				report.Readings = append(report.Readings, ArticleReading{NodeID: article.NodeID, Matches: matches})
			}
		case ArticleEmpty:
			if headingsPartial {
				report.Unread = append(report.Unread, article.NodeID)
			}
		default:
			report.Unread = append(report.Unread, article.NodeID)
		}
	})
	return report
}
