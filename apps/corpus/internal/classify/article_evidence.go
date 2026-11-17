package classify

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"danny.vn/trachuan/apps/corpus/internal/taxonomy"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ProvisionEvidence struct {
	DocumentKey, Kind, Path, Anchor, Heading, Text string
}

func ReadLabels(path string) (taxonomy.Labels, string, error) {
	var labels taxonomy.Labels
	b, err := os.ReadFile(path)
	if err != nil {
		return labels, "", fmt.Errorf("read taxonomy labels: %w", err)
	}
	if err = json.Unmarshal(b, &labels); err != nil {
		return labels, "", fmt.Errorf("decode taxonomy labels: %w", err)
	}
	if labels.Country != "vn" || labels.Release == "" || len(labels.Articles) == 0 {
		return labels, "", fmt.Errorf("taxonomy labels require country, source release and articles")
	}
	seen := map[string]bool{}
	for _, a := range labels.Articles {
		key := a.Key + "/" + a.Anchor
		if a.Key == "" || a.Anchor == "" || len(a.Codes) == 0 || seen[key] {
			return labels, "", fmt.Errorf("invalid or duplicate taxonomy label identity")
		}
		seen[key] = true
	}
	return labels, digest(b), nil
}

func LoadNodes(ctx context.Context, db *pgxpool.Pool) ([]Node, error) {
	rows, err := db.Query(ctx, `SELECT n.code,n.name,COALESCE(p.code,''),n.level,n.status='active',n.origin FROM taxonomy_node n LEFT JOIN taxonomy_node p ON p.id=n.parent_id ORDER BY n.path,n.code`)
	if err != nil {
		return nil, fmt.Errorf("read article taxonomy nodes: %w", err)
	}
	defer rows.Close()
	var nodes []Node
	for rows.Next() {
		var n Node
		if err = rows.Scan(&n.Code, &n.Name, &n.Parent, &n.Level, &n.Active, &n.Origin); err != nil {
			return nil, fmt.Errorf("decode article taxonomy node: %w", err)
		}
		nodes = append(nodes, n)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("read article taxonomy nodes: %w", err)
	}
	return nodes, nil
}

func LoadArticles(ctx context.Context, db *pgxpool.Pool, labels []taxonomy.LabelArticle) ([]Article, error) {
	keys := make([]string, 0, len(labels))
	for _, a := range labels {
		keys = append(keys, a.Key)
	}
	rows, err := db.Query(ctx, `SELECT d.key,p.kind,p.path::text,p.anchor,p.heading,p.text FROM document d JOIN provision p ON p.document_id=d.id WHERE d.key=ANY($1) ORDER BY d.key,p.preorder`, keys)
	if err != nil {
		return nil, fmt.Errorf("read selected article evidence: %w", err)
	}
	defer rows.Close()
	var provisions []ProvisionEvidence
	for rows.Next() {
		var p ProvisionEvidence
		if err = rows.Scan(&p.DocumentKey, &p.Kind, &p.Path, &p.Anchor, &p.Heading, &p.Text); err != nil {
			return nil, fmt.Errorf("decode article evidence: %w", err)
		}
		provisions = append(provisions, p)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("read selected article evidence: %w", err)
	}
	return SelectArticles(provisions, labels)
}

func SelectArticles(provisions []ProvisionEvidence, labels []taxonomy.LabelArticle) ([]Article, error) {
	selected := map[string]bool{}
	for _, a := range labels {
		key := a.Key + "/" + a.Anchor
		if selected[key] {
			return nil, fmt.Errorf("duplicate selected article")
		}
		selected[key] = true
	}
	var result []Article
	for i, p := range provisions {
		if p.Kind != "dieu" || !selected[p.DocumentKey+"/"+p.Anchor] {
			continue
		}
		a := Article{DocumentKey: p.DocumentKey, Anchor: p.Anchor, Heading: p.Heading, OwnText: p.Text, Text: p.Text}
		for j := i - 1; j >= 0 && provisions[j].DocumentKey == p.DocumentKey; j-- {
			parent := provisions[j]
			if parent.Kind == "chuong" && strings.HasPrefix(p.Path, parent.Path+".") {
				a.Chapter = parent.Path
				a.ChapterHeading = parent.Heading
				break
			}
		}
		for j := i + 1; j < len(provisions) && provisions[j].DocumentKey == p.DocumentKey && strings.HasPrefix(provisions[j].Path, p.Path+"."); j++ {
			a.Text += "\n" + provisions[j].Text
		}
		result = append(result, a)
	}
	return result, nil
}

func ValidateSelection(labels taxonomy.Labels, articles []Article, nodes []Node) error {
	texts := map[string]string{}
	for _, a := range articles {
		texts[a.DocumentKey+"/"+a.Anchor] = a.OwnText
	}
	active := map[string]bool{}
	for _, n := range nodes {
		active[n.Code] = n.Active
	}
	for _, label := range labels.Articles {
		text, ok := texts[label.Key+"/"+label.Anchor]
		if !ok || taxonomy.OwnTextSHA256(text) != label.OwnTextSHA256 {
			return fmt.Errorf("selected label does not match release article text")
		}
		for _, code := range label.Codes {
			if !active[code] {
				return fmt.Errorf("selected label names an inactive taxonomy code")
			}
		}
	}
	return nil
}
