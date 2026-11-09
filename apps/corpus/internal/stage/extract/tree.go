package extract

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/vbpl"
)

type provision struct {
	NodeID       string  `json:"node_id"`
	ParentNodeID *string `json:"parent_node_id"`
	Kind         string  `json:"kind"`
	Label        string  `json:"label"`
	Heading      string  `json:"heading"`
	Ordinal      int     `json:"ordinal"`
	Path         string  `json:"path"`
	Text         string  `json:"text"`
}

func extractTree(ctx context.Context, nodes []vbpl.Node) ([]provision, []stage.Gap, error) {
	rows := []provision{}
	gaps := []stage.Gap{}
	if len(nodes) == 0 {
		return rows, append(gaps, stage.Gap{Kind: "no_tree"}), nil
	}
	stack := append(make([]vbpl.Node, 0, len(nodes)), nodes...)
	articleCount := 0
	for len(stack) > 0 {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		top := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if nodeKind(top.Level, top.PType) == "dieu" {
			articleCount++
		}
		stack = append(stack, top.Children...)
	}
	paths := make(map[string]int)
	articles := make(map[int]bool)
	maxArticle := 0
	var walk func([]vbpl.Node, *string, string, string, string) error
	walk = func(siblings []vbpl.Node, parent *string, chapter, article, clause string) error {
		for i, node := range siblings {
			if err := ctx.Err(); err != nil {
				return err
			}
			kind := nodeKind(node.Level, node.PType)
			label, heading, segment, number := titleParts(kind, node.Title, i+1)
			path := segment
			switch kind {
			case "muc":
				if chapter != "" {
					path = chapter + "/" + segment
				}
			case "khoan":
				if article != "" {
					path = article + "/" + segment
				}
			case "diem":
				if clause != "" {
					path = clause + "/" + segment
				} else if article != "" {
					path = article + "/" + segment
				}
			case "other":
				if parent != nil {
					path = "other-" + strconv.Itoa(i+1)
				}
			}
			base := path
			paths[base]++
			if paths[base] > 1 {
				path = fmt.Sprintf("%s~%d", base, paths[base])
			}
			text := textFromHTML(node.Content)
			rows = append(rows, provision{NodeID: node.ID, ParentNodeID: parent, Kind: kind, Label: label, Heading: heading, Ordinal: i + 1, Path: path, Text: text})
			if kind == "other" {
				gaps = append(gaps, stage.Gap{Kind: "unknown_node_kind", Detail: node.ID})
			}
			if kind == "dieu" {
				if strings.TrimSpace(text) == "" {
					gaps = append(gaps, stage.Gap{Kind: "empty_text", Detail: node.ID})
				}
				if number > articleCount+100 {
					gaps = append(gaps, stage.Gap{Kind: "article_number_out_of_range", Detail: strings.TrimPrefix(label, "Điều ")})
				} else if number > 0 {
					articles[number] = true
					if number > maxArticle {
						maxArticle = number
					}
				}
			}
			childChapter, childArticle, childClause := chapter, article, clause
			switch kind {
			case "chuong":
				childChapter = path
			case "dieu":
				childArticle = path
				childClause = ""
			case "khoan":
				childClause = path
			}
			id := node.ID
			if err := walk(node.Children, &id, childChapter, childArticle, childClause); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk(nodes, nil, "", "", ""); err != nil {
		return nil, nil, err
	}
	sequenceGaps := 0
	for i := 1; i < maxArticle && sequenceGaps < 100; i++ {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		if !articles[i] {
			start := i
			for i+1 < maxArticle && !articles[i+1] {
				i++
			}
			detail := strconv.Itoa(start)
			if i > start {
				detail += "-" + strconv.Itoa(i)
			}
			gaps = append(gaps, stage.Gap{Kind: "article_sequence", Detail: detail})
			sequenceGaps++
		}
	}
	return rows, gaps, nil
}
