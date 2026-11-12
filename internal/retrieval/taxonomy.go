package retrieval

import (
	"context"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// TaxonomyRequest selects a subtree or a folded name search.
type TaxonomyRequest struct {
	Node  string
	Depth int
	Q     string
}

// TaxonomyResult contains the release's served nodes.
type TaxonomyResult struct {
	Release   string      `json:"release"`
	Nodes     []NodeEntry `json:"nodes"`
	Truncated bool        `json:"truncated"`
}

// NodeEntry describes a served taxonomy node.
type NodeEntry struct {
	Code        string      `json:"code"`
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Level       int         `json:"level"`
	Origin      string      `json:"origin"`
	Provisions  int         `json:"provisions"`
	Documents   int         `json:"documents"`
	Children    []NodeEntry `json:"children"`
}

// NodeDocument describes a document associated with a node's subtree.
type NodeDocument struct {
	Key         string `json:"key"`
	Number      string `json:"number"`
	Title       string `json:"title"`
	Type        string `json:"type"`
	StatusClass string `json:"status_class"`
	IsCurrent   bool   `json:"is_current"`
	Provisions  int    `json:"provisions"`
}

// NodePage contains a node, its ancestors and its most represented documents.
type NodePage struct {
	Release   string         `json:"release"`
	Node      NodeEntry      `json:"node"`
	Path      []NodeEntry    `json:"path"`
	Documents []NodeDocument `json:"documents"`
}

var taxonomyCodePattern = regexp.MustCompile(`^[1-9][0-9]{0,2}(\.[1-9][0-9]{0,3}(\.[pcx][1-9][0-9]{0,3}(\.[cmx][1-9][0-9]{0,3})?)?)?$`)

func validTaxonomyCode(code string) bool {
	return taxonomyCodePattern.MatchString(code)
}
func foldTaxonomy(s string) string {
	var b strings.Builder
	for _, r := range norm.NFD.String(strings.ToLower(norm.NFC.String(s))) {
		if unicode.Is(unicode.Mn, r) {
			continue
		}
		if r == 'đ' {
			r = 'd'
		}
		b.WriteRune(r)
	}
	return b.String()
}

type taxonomyRow struct {
	id, parent int64
	entry      NodeEntry
}

// Taxonomy returns a served subtree or a flat folded search.
func (e *Engine) Taxonomy(ctx context.Context, req TaxonomyRequest) (TaxonomyResult, error) {
	result := TaxonomyResult{Release: e.release.Rel, Nodes: []NodeEntry{}}
	if req.Node != "" && !validTaxonomyCode(req.Node) || utf8.RuneCountInString(req.Q) > 100 {
		return result, ErrInvalidRequest
	}
	q := strings.TrimSpace(req.Q)
	if q == "" && (req.Depth < 0 || req.Depth > 3) {
		return result, ErrInvalidRequest
	}
	if e.release.SchemaVersion == 4 {
		if req.Node != "" {
			return result, ErrNotFound
		}
		return result, nil
	}
	ctx, cancel := context.WithTimeout(ctx, e.deadline())
	defer cancel()
	tx, err := e.metadataTx(ctx)
	if err != nil {
		return result, err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	rows, err := loadTaxonomy(ctx, tx)
	if err != nil {
		return result, err
	}
	anchor := int64(0)
	if req.Node != "" {
		found := false
		for _, row := range rows {
			if row.entry.Code == req.Node {
				anchor = row.id
				found = true
				break
			}
		}
		if !found {
			return result, ErrNotFound
		}
	}
	children := map[int64][]taxonomyRow{}
	for _, row := range rows {
		children[row.parent] = append(children[row.parent], row)
	}
	if q != "" {
		needle := foldTaxonomy(q)
		var walk func(int64)
		walk = func(id int64) {
			for _, row := range children[id] {
				if strings.Contains(foldTaxonomy(row.entry.Name), needle) || strings.Contains(foldTaxonomy(row.entry.Description), needle) {
					if len(result.Nodes) < 500 {
						result.Nodes = append(result.Nodes, row.entry)
					} else {
						result.Truncated = true
					}
				}
				walk(row.id)
			}
		}
		if anchor != 0 {
			for _, row := range rows {
				if row.id == anchor && (strings.Contains(foldTaxonomy(row.entry.Name), needle) || strings.Contains(foldTaxonomy(row.entry.Description), needle)) {
					result.Nodes = append(result.Nodes, row.entry)
					break
				}
			}
		}
		walk(anchor)
	} else {
		depth := req.Depth
		if depth == 0 {
			depth = 1
		}
		selected := map[int64]bool{}
		queue := []int64{anchor}
		for level := 0; level < depth && len(queue) > 0; level++ {
			next := []int64{}
			for _, id := range queue {
				for _, row := range children[id] {
					if len(selected) < 500 {
						selected[row.id] = true
						next = append(next, row.id)
					} else {
						result.Truncated = true
					}
				}
			}
			queue = next
		}
		var build func(int64) []NodeEntry
		build = func(id int64) []NodeEntry {
			out := []NodeEntry{}
			for _, row := range children[id] {
				if selected[row.id] {
					entry := row.entry
					entry.Children = build(row.id)
					out = append(out, entry)
				}
			}
			return out
		}
		result.Nodes = build(anchor)
	}
	if err = tx.Commit(ctx); err != nil {
		return result, databaseFailure("commit taxonomy", err)
	}
	return result, nil
}

// TaxonomyNode returns a served node and its leading documents.
func (e *Engine) TaxonomyNode(ctx context.Context, code string) (NodePage, error) {
	result := NodePage{Release: e.release.Rel, Path: []NodeEntry{}, Documents: []NodeDocument{}}
	if !validTaxonomyCode(code) {
		return result, ErrInvalidRequest
	}
	if e.release.SchemaVersion == 4 {
		return result, ErrNotFound
	}
	ctx, cancel := context.WithTimeout(ctx, e.deadline())
	defer cancel()
	tx, err := e.metadataTx(ctx)
	if err != nil {
		return result, err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	rows, err := loadTaxonomy(ctx, tx)
	if err != nil {
		return result, err
	}
	byID := map[int64]taxonomyRow{}
	var target taxonomyRow
	for _, row := range rows {
		byID[row.id] = row
		if row.entry.Code == code {
			target = row
		}
	}
	if target.id == 0 {
		return result, ErrNotFound
	}
	result.Node = target.entry
	for _, row := range rows {
		if row.parent == target.id {
			result.Node.Children = append(result.Node.Children, row.entry)
		}
	}
	if result.Node.Children == nil {
		result.Node.Children = []NodeEntry{}
	}
	for id := target.parent; id != 0; id = byID[id].parent {
		result.Path = append([]NodeEntry{byID[id].entry}, result.Path...)
	}
	result.Documents, err = taxonomyDocuments(ctx, tx, target.id)
	if err != nil {
		return result, err
	}
	if err = tx.Commit(ctx); err != nil {
		return result, databaseFailure("commit taxonomy node", err)
	}
	return result, nil
}
